package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func signedRefundWebhookBody(t *testing.T, orderID, secret, status string, amount, refundedAmount int) ([]byte, string) {
	t.Helper()
	txn := map[string]any{
		"id":          9001,
		"status":      status,
		"referenceId": orderID,
		"amount":      amount,
		"currency":    "CHF",
	}
	if refundedAmount > 0 {
		txn["refundedAmount"] = refundedAmount
	}
	body, err := json.Marshal(map[string]any{"transaction": txn})
	require.NoError(t, err)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return body, hex.EncodeToString(mac.Sum(nil))
}

// TestPayrexxWebhook_RefundReconcile proves refund webhooks are reconciled
// against the cumulative refunded amount rather than dropped by the IsSuccess
// gate: partial refunds stay partial, and refunds we initiated aren't re-counted.
func TestPayrexxWebhook_RefundReconcile(t *testing.T) {
	env := newRLSEnv(t)
	enc := newTestEncryptor(t)
	tn := env.configurePayrexxTenant(t, enc, "wh-refund", "instance-r", "api-r", "secret-r")
	router := env.buildWebhookRouter(t, enc)

	t.Run("external full refund records a reconciling refund + flips status", func(t *testing.T) {
		orderID := nanoid.New()
		env.privExec(t,
			`INSERT INTO "order" (id, total_cents, status, origin, payrexx_transaction_id, tenant_id)
			 VALUES ($1, 1000, 'paid', 'shop', 9001, $2)`,
			orderID, tn.id,
		)

		body, sig := signedRefundWebhookBody(t, orderID, tn.webhookSecret, "refunded", 1000, 1000)
		rec := postWebhook(t, router, tn.webhookToken, body, sig)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.Equal(t, "refunded", env.orderStatus(t, orderID))
		require.Equal(t, 1, env.countRefunds(t, orderID))

		var status, kind string
		require.NoError(t, env.priv.QueryRow(
			`SELECT status, kind FROM refund WHERE order_id = $1`, orderID,
		).Scan(&status, &kind))
		require.Equal(t, "succeeded", status)
		require.Equal(t, "arbitrary", kind)
	})

	t.Run("external partial refund records only the refunded portion", func(t *testing.T) {
		orderID := nanoid.New()
		env.privExec(t,
			`INSERT INTO "order" (id, total_cents, status, origin, payrexx_transaction_id, tenant_id)
			 VALUES ($1, 1000, 'paid', 'shop', 9004, $2)`,
			orderID, tn.id,
		)

		body, sig := signedRefundWebhookBody(t, orderID, tn.webhookSecret, "partially-refunded", 1000, 300)
		rec := postWebhook(t, router, tn.webhookToken, body, sig)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.Equal(t, "partially_refunded", env.orderStatus(t, orderID))
		require.Equal(t, 1, env.countRefunds(t, orderID))

		var amount int64
		require.NoError(t, env.priv.QueryRow(
			`SELECT amount_cents FROM refund WHERE order_id = $1`, orderID,
		).Scan(&amount))
		require.Equal(t, int64(300), amount)
	})

	t.Run("a refund we already recorded is not double-counted", func(t *testing.T) {
		orderID := nanoid.New()
		env.privExec(t,
			`INSERT INTO "order" (id, total_cents, status, origin, payrexx_transaction_id, tenant_id)
			 VALUES ($1, 1000, 'partially_refunded', 'shop', 9002, $2)`,
			orderID, tn.id,
		)
		env.privExec(t,
			`INSERT INTO refund (id, tenant_id, order_id, amount_cents, kind, method, actor_type, actor_id, status)
			 VALUES ($1, $2, $3, 500, 'line', 'twint', 'operator', 'op1', 'succeeded')`,
			nanoid.New(), tn.id, orderID,
		)

		body, sig := signedRefundWebhookBody(t, orderID, tn.webhookSecret, "partially-refunded", 1000, 500)
		rec := postWebhook(t, router, tn.webhookToken, body, sig)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.Equal(t, 1, env.countRefunds(t, orderID))
	})

	t.Run("partial refund without refundedAmount is skipped", func(t *testing.T) {
		orderID := nanoid.New()
		env.privExec(t,
			`INSERT INTO "order" (id, total_cents, status, origin, payrexx_transaction_id, tenant_id)
			 VALUES ($1, 1000, 'paid', 'shop', 9003, $2)`,
			orderID, tn.id,
		)

		body, sig := signedRefundWebhookBody(t, orderID, tn.webhookSecret, "partially-refunded", 1000, 0)
		rec := postWebhook(t, router, tn.webhookToken, body, sig)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.Equal(t, "paid", env.orderStatus(t, orderID))
		require.Equal(t, 0, env.countRefunds(t, orderID))
	})
}
