package integration

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"backend/internal/database"
	"backend/internal/generated/ent/order"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// These tests run service/repository operations through the real RLS path: a
// per-request tenant transaction on the comanda_app pool with app.tenant_id
// set. They reproduce production and would have caught the nested-Ent-tx
// regressions (a nested tx opens on the static pool with no GUC → RLS blocks).

// seedTenantWithProducts seeds a tenant + a category + N simple products via the
// privileged pool and returns the tenant id and product ids.
func (e *rlsEnv) seedTenantWithProducts(t *testing.T, slug string, n int) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	tenantID := e.seedTenant(t, slug)
	catID := uuid.Must(uuid.NewV7())
	e.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'Cat', $2)`, catID, tenantID)
	ids := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		pid := uuid.Must(uuid.NewV7())
		e.privExec(t,
			`INSERT INTO product (id, category_id, type, name, price_cents, is_active, tenant_id)
			 VALUES ($1, $2, 'simple', $3, 100, true, $4)`,
			pid, catID, "P", tenantID,
		)
		ids[i] = pid
	}
	return tenantID, ids
}

func appClientFor(db *sql.DB) *database.AppClient {
	return &database.AppClient{DB: db}
}

// H1: volunteer campaign create + redeem must not 500 inside the request tx.
func TestTenantRequest_VolunteerCreateAndRedeem(t *testing.T) {
	env := newRLSEnv(t)
	tenantID, productIDs := env.seedTenantWithProducts(t, "vol-"+uuid.NewString()[:8], 1)

	// Seed a station device + assign the campaign product to it (so the redeem
	// station-match check passes).
	stationID := uuid.Must(uuid.NewV7())
	env.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id)
		 VALUES ($1, 'St', $2, 'STATION', 'approved', $3)`,
		stationID, "stationkey-"+uuid.NewString()[:8], tenantID,
	)
	env.privExec(t,
		`INSERT INTO device_product (device_id, product_id, tenant_id) VALUES ($1, $2, $3)`,
		stationID, productIDs[0], tenantID,
	)

	ctx, client, cleanup := env.appRequestTx(t, tenantID)
	defer cleanup()

	repos := NewRepositories(client)
	cfg := TestConfig()
	stations := service.NewStationService(cfg, client, repos.Device, repos.DeviceProduct, repos.OrderLine, repos.OrderRedemption, repos.Idempotency)
	svc := service.NewVolunteerService(client, repos.VolunteerCampaign, repos.VolunteerRedeem, repos.Order, repos.OrderLine, repos.OrderPayment, stations)

	campaign, err := svc.CreateCampaign(ctx, service.CreateVolunteerCampaignInput{
		Name:           "Crew Meal",
		MaxRedemptions: 5,
		Products:       []repository.VolunteerCampaignProductInput{{ProductID: productIDs[0], Quantity: 1}},
	})
	require.NoError(t, err, "CreateCampaign must not error inside the request tx (no nested tx / RLS block)")
	require.NotNil(t, campaign)

	// Update must also work (was an H1 site).
	max := 10
	_, err = svc.UpdateCampaign(ctx, campaign.ID, service.UpdateVolunteerCampaignInput{
		Name:           "Crew Meal v2",
		Status:         campaign.Status,
		MaxRedemptions: &max,
		Products:       []repository.VolunteerCampaignProductInput{{ProductID: productIDs[0], Quantity: 2}},
	})
	require.NoError(t, err, "UpdateCampaign must not error inside the request tx")

	res, err := svc.RedeemSharedQR(ctx, campaign.ClaimToken, stationID, "idem-"+uuid.NewString()[:8])
	require.NoError(t, err, "RedeemSharedQR must not error inside the request tx")
	require.NotEqual(t, uuid.Nil, res.OrderID)
}

// H2: replacing station products must not 500 inside the request tx.
func TestTenantRequest_SetStationProducts(t *testing.T) {
	env := newRLSEnv(t)
	tenantID, productIDs := env.seedTenantWithProducts(t, "sta-"+uuid.NewString()[:8], 2)

	stationID := uuid.Must(uuid.NewV7())
	env.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id)
		 VALUES ($1, 'St', $2, 'STATION', 'approved', $3)`,
		stationID, "stationkey-"+uuid.NewString()[:8], tenantID,
	)

	ctx, client, cleanup := env.appRequestTx(t, tenantID)
	defer cleanup()

	repos := NewRepositories(client)
	stations := service.NewStationService(TestConfig(), client, repos.Device, repos.DeviceProduct, repos.OrderLine, repos.OrderRedemption, repos.Idempotency)

	require.NoError(t, stations.SetStationProducts(ctx, stationID, productIDs),
		"SetStationProducts (ReplaceForDevice) must not error inside the request tx")

	got, err := stations.ListStationProductIDs(ctx, stationID)
	require.NoError(t, err)
	require.ElementsMatch(t, productIDs, got)

	// Replace again with a subset — exercises delete+insert path once more.
	require.NoError(t, stations.SetStationProducts(ctx, stationID, productIDs[:1]))
	got, err = stations.ListStationProductIDs(ctx, stationID)
	require.NoError(t, err)
	require.ElementsMatch(t, productIDs[:1], got)
}

// H2: POS cash payment (setPosPayment) must not 500 inside the request tx.
func TestTenantRequest_PosPayment(t *testing.T) {
	env := newRLSEnv(t)
	tenantID, _ := env.seedTenantWithProducts(t, "pos-"+uuid.NewString()[:8], 0)

	orderID := uuid.Must(uuid.NewV7())
	env.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, tenant_id)
		 VALUES ($1, 250, 'pending', 'pos', $2)`,
		orderID, tenantID,
	)

	ctx, client, cleanup := env.appRequestTx(t, tenantID)
	defer cleanup()

	orders := repository.NewOrderRepository(client)
	require.NoError(t, orders.SetPosPaymentCash(ctx, orderID, nil, 250),
		"SetPosPaymentCash (setPosPayment) must not error inside the request tx")

	o, err := orders.GetByID(ctx, orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusPaid, o.Status)
}

// M1: settings system_enabled toggles round-trip per tenant.
func TestTenantRequest_SettingsSystemEnabledRoundTrip(t *testing.T) {
	env := newRLSEnv(t)
	tenantID := env.seedTenant(t, "set-"+uuid.NewString()[:8])

	ctx, client, cleanup := env.appRequestTx(t, tenantID)
	defer cleanup()

	settingsRepo := repository.NewSettingsRepository(client)
	svc := service.NewSettingsService(settingsRepo, repository.NewJetonRepository(client), repository.NewProductRepository(client))

	require.NoError(t, svc.SetSystemEnabled(ctx, false))
	enabled, err := svc.IsSystemEnabled(ctx)
	require.NoError(t, err)
	require.False(t, enabled, "system_enabled should round-trip to false on the per-tenant settings row")

	require.NoError(t, svc.SetSystemEnabled(ctx, true))
	enabled, err = svc.IsSystemEnabled(ctx)
	require.NoError(t, err)
	require.True(t, enabled)
}

// captureEmail records the most recent receipt email.
type captureEmail struct {
	mu   sync.Mutex
	data *service.ReceiptEmailData
}

func (c *captureEmail) SendInviteEmail(context.Context, string, string, time.Time) error { return nil }
func (c *captureEmail) SendOTPEmail(context.Context, string, string, service.OTPType) error {
	return nil
}
func (c *captureEmail) SendReceiptEmail(_ context.Context, _ string, data service.ReceiptEmailData) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := data
	c.data = &d
	return nil
}
func (c *captureEmail) get() *service.ReceiptEmailData {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data
}

// M2: the receipt goroutine must load order line items (it runs detached, so it
// opens its own tenant tx; without the GUC it would see zero lines).
func TestTenantRequest_ReceiptHasLineItems(t *testing.T) {
	env := newRLSEnv(t)
	tenantID, productIDs := env.seedTenantWithProducts(t, "rcpt-"+uuid.NewString()[:8], 1)

	// Seed a committed order + line + contact email via the privileged pool so
	// the detached receipt tx can read it.
	orderID := uuid.Must(uuid.NewV7())
	email := "guest@example.com"
	env.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, contact_email, tenant_id)
		 VALUES ($1, 100, 'pending', 'shop', $2, $3)`,
		orderID, email, tenantID,
	)
	env.privExec(t,
		`INSERT INTO order_line (id, order_id, line_type, product_id, title, quantity, unit_price_cents, tenant_id)
		 VALUES ($1, $2, 'simple', $3, 'Cola', 2, 100, $4)`,
		uuid.Must(uuid.NewV7()), orderID, productIDs[0], tenantID,
	)

	capture := &captureEmail{}
	cfg := TestConfig()
	ctx, client, cleanup := env.appRequestTx(t, tenantID)
	defer cleanup()

	repos := NewRepositories(client)
	svc := service.NewPaymentService(
		cfg,
		repos.Order,
		repos.OrderLine,
		repos.OrderPayment,
		NewProductSvc(repos),
		repos.MenuSlot,
		repos.Inventory,
		nil,
		capture,
		repos.Settings,
		appClientFor(env.app),
		zap.NewNop(),
	)

	require.NoError(t, svc.MarkOrderPaidByPayrexx(ctx, orderID, 111, 222, &email))

	// sendReceipt is fire-and-forget; wait for the capture.
	var data *service.ReceiptEmailData
	require.Eventually(t, func() bool {
		data = capture.get()
		return data != nil
	}, 5*time.Second, 25*time.Millisecond, "receipt email was never sent")

	require.NotEmpty(t, data.Items, "receipt must include order line items (tenant GUC reattached in the detached tx)")
	require.Equal(t, "Cola", data.Items[0].Title)
}
