package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/api"
	"backend/internal/crypto"
	"backend/internal/generated/ent/order"
	"backend/internal/payment"
	"backend/internal/repository"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type payrexxTestTenant struct {
	id            string
	webhookToken  string
	webhookSecret string
	orderID       string
}

func newTestEncryptor(t *testing.T) *crypto.Encryptor {
	t.Helper()
	key := make([]byte, crypto.KeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)
	enc, err := crypto.NewEncryptor(key)
	require.NoError(t, err)
	return enc
}

// configurePayrexxTenant writes encrypted credentials under the tenant's own
// RLS context, exercising the production write path.
func (e *rlsEnv) configurePayrexxTenant(t *testing.T, enc *crypto.Encryptor, slug, instance, apiSecret, webhookSecret string) payrexxTestTenant {
	t.Helper()
	tenantID := e.seedTenant(t, slug+"-"+randSuffix())

	orderID := nanoid.New()
	e.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, tenant_id)
		 VALUES ($1, 1000, 'pending', 'shop', $2)`,
		orderID, tenantID,
	)

	apiEnc, err := enc.EncryptString(apiSecret)
	require.NoError(t, err)
	whEnc, err := enc.EncryptString(webhookSecret)
	require.NoError(t, err)

	token := nanoid.New() + nanoid.New() + nanoid.New() + nanoid.New()
	token = token[:43]

	ctx, _, cleanup := e.appRequestTx(t, tenantID)
	defer cleanup()
	repo := repository.NewSettingsRepository(nil)
	require.NoError(t, repo.UpsertPayrexxCredentials(ctx, repository.PayrexxCredentials{
		Instance:         instance,
		APISecretEnc:     apiEnc,
		WebhookSecretEnc: whEnc,
		WebhookToken:     token,
	}))

	return payrexxTestTenant{id: tenantID, webhookToken: token, webhookSecret: webhookSecret, orderID: orderID}
}

func signedWebhookBody(t *testing.T, orderID string, secret string) ([]byte, string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"transaction": map[string]any{
			"id":          4242,
			"status":      "confirmed",
			"referenceId": orderID,
			"amount":      1000,
			"currency":    "CHF",
		},
	})
	require.NoError(t, err)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return body, hex.EncodeToString(mac.Sum(nil))
}

func (e *rlsEnv) buildWebhookRouter(t *testing.T, enc *crypto.Encryptor) http.Handler {
	t.Helper()
	appClient := appClientFor(e.app)
	// The actual tx is rebound per request by the handler via beginWebhookTenantTx.
	repos := NewRepositories(appClient.Client)
	factory := payment.NewFactory(repos.Settings, enc)

	payments := service.NewPaymentService(
		TestConfig(),
		factory,
		enc,
		repos.Order,
		repos.OrderLine,
		repos.OrderPayment,
		NewProductSvc(repos),
		repos.MenuSlot,
		repos.Inventory,
		nil,
		nil,
		repos.Settings,
		appClient,
		zap.NewNop(),
	)

	h := api.NewWebhookHandlers(payments, e.privClient(), appClient, factory, zap.NewNop())

	r := chi.NewRouter()
	r.Post("/v1/payments/webhooks/payrexx/{tenantToken}", func(w http.ResponseWriter, req *http.Request) {
		h.HandlePayrexxWebhook(w, req, chi.URLParam(req, "tenantToken"))
	})
	return r
}

func (e *rlsEnv) orderStatus(t *testing.T, orderID string) string {
	t.Helper()
	var status string
	require.NoError(t, e.priv.QueryRow(`SELECT status FROM "order" WHERE id = $1`, orderID).Scan(&status))
	return status
}

func postWebhook(t *testing.T, router http.Handler, token string, body []byte, signature string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhooks/payrexx/"+token, bytes.NewReader(body))
	req.Header.Set("X-Webhook-Signature", signature)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestPayrexxWebhook_CrossTenantIsolation proves a webhook is honoured only when
// posted to the owning tenant's token AND signed with that tenant's secret, so
// A's signed body posted to B's token can never be cross-attributed.
func TestPayrexxWebhook_CrossTenantIsolation(t *testing.T) {
	env := newRLSEnv(t)
	enc := newTestEncryptor(t)

	a := env.configurePayrexxTenant(t, enc, "wh-tenant-a", "instance-a", "api-a", "secret-a")
	b := env.configurePayrexxTenant(t, enc, "wh-tenant-b", "instance-b", "api-b", "secret-b")
	router := env.buildWebhookRouter(t, enc)

	t.Run("correct token + correct secret -> order paid", func(t *testing.T) {
		body, sig := signedWebhookBody(t, a.orderID, a.webhookSecret)
		rec := postWebhook(t, router, a.webhookToken, body, sig)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, string(order.StatusPaid), env.orderStatus(t, a.orderID))
	})

	t.Run("A's signed body posted to B's token -> 401, no cross-attribution", func(t *testing.T) {
		body, sigA := signedWebhookBody(t, a.orderID, a.webhookSecret)
		// Posted to B's token: B's webhook secret must verify, and it cannot.
		rec := postWebhook(t, router, b.webhookToken, body, sigA)
		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		// B's order remains pending; A's order stays paid (untouched by B's request).
		require.Equal(t, string(order.StatusPending), env.orderStatus(t, b.orderID))
		require.Equal(t, string(order.StatusPaid), env.orderStatus(t, a.orderID))
	})

	t.Run("unknown token -> 404", func(t *testing.T) {
		body, sig := signedWebhookBody(t, a.orderID, a.webhookSecret)
		rec := postWebhook(t, router, "this-token-belongs-to-no-tenant", body, sig)
		require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})

	t.Run("right token, wrong signature -> 401", func(t *testing.T) {
		body, _ := signedWebhookBody(t, b.orderID, b.webhookSecret)
		rec := postWebhook(t, router, b.webhookToken, body, "deadbeef")
		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		require.Equal(t, string(order.StatusPending), env.orderStatus(t, b.orderID))
	})
}

// TestPayrexxCredentials_NeverStorePlaintext asserts the DB row contains only
// ciphertext: the plaintext secrets must not appear in any settings column, and
// the stored ciphertext must decrypt back to the originals.
func TestPayrexxCredentials_NeverStorePlaintext(t *testing.T) {
	env := newRLSEnv(t)
	enc := newTestEncryptor(t)

	const apiSecret = "PLAINTEXT_API_SECRET_zzz"
	const webhookSecret = "PLAINTEXT_WEBHOOK_SECRET_zzz"
	tn := env.configurePayrexxTenant(t, enc, "wh-tenant-plain", "instance-x", apiSecret, webhookSecret)

	// Read the raw persisted row via the privileged pool (RLS bypassed).
	var apiEnc, whEnc, instance string
	require.NoError(t, env.priv.QueryRow(
		`SELECT payrexx_instance, payrexx_api_secret_enc, payrexx_webhook_secret_enc
		 FROM settings WHERE tenant_id = $1`, tn.id,
	).Scan(&instance, &apiEnc, &whEnc))

	require.NotContains(t, apiEnc, apiSecret, "api secret plaintext leaked into the DB")
	require.NotContains(t, whEnc, webhookSecret, "webhook secret plaintext leaked into the DB")
	require.NotEqual(t, apiSecret, apiEnc)
	require.NotEqual(t, webhookSecret, whEnc)

	// And the ciphertext decrypts back to the originals.
	gotAPI, err := enc.DecryptString(apiEnc)
	require.NoError(t, err)
	require.Equal(t, apiSecret, gotAPI)
	gotWH, err := enc.DecryptString(whEnc)
	require.NoError(t, err)
	require.Equal(t, webhookSecret, gotWH)
	_ = instance
}

// TestPayrexxWebhook_TokenResolutionIsPrivileged proves the token->tenant lookup
// works across tenants (the privileged client bypasses RLS), which is what makes
// the pre-context resolution possible.
func TestPayrexxWebhook_TokenResolutionIsPrivileged(t *testing.T) {
	env := newRLSEnv(t)
	enc := newTestEncryptor(t)
	a := env.configurePayrexxTenant(t, enc, "wh-tenant-res-a", "instance-a", "api-a", "secret-a")
	b := env.configurePayrexxTenant(t, enc, "wh-tenant-res-b", "instance-b", "api-b", "secret-b")

	// Factory.For under each tenant's RLS context must resolve that tenant's
	// instance, never the other's.
	for _, tc := range []struct {
		tn       payrexxTestTenant
		instance string
	}{{a, "instance-a"}, {b, "instance-b"}} {
		ctx, _, cleanup := env.appRequestTx(t, tc.tn.id)
		repos := NewRepositories(nil)
		factory := payment.NewFactory(repos.Settings, enc)
		_, err := factory.For(ctx)
		require.NoError(t, err)
		creds, err := repos.Settings.GetPayrexxCredentials(ctx)
		require.NoError(t, err)
		require.Equal(t, tc.instance, creds.Instance)
		cleanup()
	}
}
