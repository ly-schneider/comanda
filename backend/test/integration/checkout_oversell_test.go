package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"backend/internal/api"
	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/product"
	"backend/internal/repository"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// privRequestTx mirrors the per-request tenant tx against the privileged test
// pool: its own connection + tx so a FOR UPDATE row lock actually blocks a
// second concurrent tx (a single shared Ent client cannot — one pgx connection
// can't run two txs). Cleanup commits, releasing the lock.
func privRequestTx(t *testing.T, tdb *TestDB, tenantID string) (context.Context, *ent.Client, func()) {
	t.Helper()
	ctx := context.Background()
	conn, err := tdb.DB.Conn(ctx)
	require.NoError(t, err)
	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		t.Fatalf("set tenant guc: %v", err)
	}
	client := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
	enttenant.Register(client)
	ctx = repository.ContextWithClient(ctx, client)
	ctx = repository.ContextWithRequestTx(ctx, tx)
	ctx = repository.ContextWithTenantID(ctx, tenantID)
	ctx = enttenant.WithTenant(ctx, tenantID)
	cleanup := func() {
		_ = tx.Commit()
		_ = conn.Close()
	}
	return ctx, client, cleanup
}

// TestPaymentService_ConcurrentCheckoutNoOversell proves the FOR UPDATE product
// lock in PrepareAndCreateOrder serializes two concurrent checkouts of the last
// unit: without it both read available=1, both pass, both reserve -1 → oversell
// (stock -1). With it, the second blocks on the lock until the first commits,
// then reads available=0 and is rejected. Exactly one checkout wins; stock
// floors at 0.
func TestPaymentService_ConcurrentCheckoutNoOversell(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	cfg := TestConfig()

	setupRepos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(setupRepos)
	category := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	fixtures.AddInventory(cola.ID, 1, inventoryledger.ReasonOpeningBalance) // exactly one in stock

	run := func() error {
		ctx, client, cleanup := privRequestTx(t, tdb, DefaultTestTenantID)
		defer cleanup()
		repos := NewRepositories(client)
		svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
		_, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: cola.ID, Quantity: 1}},
		}, nil, nil)
		return err
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = run()
		}(i)
	}
	wg.Wait()

	successes, soldOut := 0, 0
	for _, e := range errs {
		if e == nil {
			successes++
			continue
		}
		var ins *service.InsufficientInventoryError
		if errors.As(e, &ins) {
			soldOut++
		} else {
			t.Fatalf("unexpected checkout error: %v", e)
		}
	}
	require.Equal(t, 1, successes, "exactly one checkout may win the last unit")
	require.Equal(t, 1, soldOut, "the loser must get a typed out-of-stock error")

	stock, err := setupRepos.Inventory.GetCurrentStock(tenantCtx(), cola.ID)
	require.NoError(t, err)
	require.Equal(t, 0, stock, "stock must floor at 0 — no oversell")
}

// TestPaymentService_InsufficientInventoryTypedError checks the typed error the
// out-of-stock UX depends on: it carries the offending product id, requested and
// available counts.
func TestPaymentService_InsufficientInventoryTypedError(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	svc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	ctx := tenantCtx()

	category := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	fixtures.AddInventory(cola.ID, 2, inventoryledger.ReasonOpeningBalance)

	_, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
		Items: []service.CheckoutItemInput{{ProductID: cola.ID, Quantity: 5}},
	}, nil, nil)

	var ins *service.InsufficientInventoryError
	require.ErrorAs(t, err, &ins)
	require.Len(t, ins.Items, 1)
	require.Equal(t, cola.ID, ins.Items[0].ProductID)
	require.Equal(t, 5, ins.Items[0].Requested)
	require.Equal(t, 2, ins.Items[0].Available)
}

// TestPaymentService_MarkOrderPaidIdempotent proves a duplicate/retried success
// webhook doesn't double-charge (a second order_payment) or error, and that a
// webhook for a non-pending, non-paid order is rejected rather than silently
// resurrecting it.
func TestPaymentService_MarkOrderPaidIdempotent(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	svc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	ctx := tenantCtx()

	ord := fixtures.CreateOrder(1000, order.StatusPending, order.OriginShop)

	require.NoError(t, svc.MarkOrderPaidByPayrexx(ctx, ord.ID, 111, 222, nil))
	// Duplicate webhook: idempotent no-op, no error.
	require.NoError(t, svc.MarkOrderPaidByPayrexx(ctx, ord.ID, 111, 222, nil))

	updated, err := repos.Order.GetByID(ctx, ord.ID)
	require.NoError(t, err)
	require.Equal(t, order.StatusPaid, updated.Status)

	var payments int
	require.NoError(t, tdb.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM order_payment WHERE order_id = $1`, ord.ID).Scan(&payments))
	require.Equal(t, 1, payments, "duplicate webhook must not create a second payment")

	cancelled := fixtures.CreateOrder(500, order.StatusCancelled, order.OriginShop)
	require.Error(t, svc.MarkOrderPaidByPayrexx(ctx, cancelled.ID, 1, 2, nil),
		"a success webhook for a non-pending order must be rejected, not resurrect it")
}

// TestOrderRepo_ListStalePending covers the backstop sweeper's enumeration: it
// returns pending orders older than the cutoff across ALL tenants (run on the
// privileged, RLS-bypassing client), excluding fresh and non-pending ones.
func TestOrderRepo_ListStalePending(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := context.Background()

	t2 := nanoid.New()
	_, err := tdb.DB.ExecContext(ctx, `INSERT INTO tenant (id, slug, name) VALUES ($1, $2, $3)`, t2, "stale-"+t2, "T2")
	require.NoError(t, err)

	insOrder := func(tenant string, status order.Status, createdAt time.Time) string {
		id := nanoid.New()
		_, err := tdb.DB.ExecContext(ctx,
			`INSERT INTO "order" (id, total_cents, status, origin, tenant_id, created_at) VALUES ($1, 100, $2, 'pos', $3, $4)`,
			id, string(status), tenant, createdAt)
		require.NoError(t, err)
		return id
	}
	old := time.Now().Add(-10 * time.Minute)
	staleA := insOrder(DefaultTestTenantID, order.StatusPending, old)
	staleB := insOrder(t2, order.StatusPending, old) // different tenant
	freshPending := insOrder(DefaultTestTenantID, order.StatusPending, time.Now())
	paidOld := insOrder(DefaultTestTenantID, order.StatusPaid, old)

	rows, err := repos.Order.ListStalePending(ctx, tdb.Client, time.Now().Add(-5*time.Minute))
	require.NoError(t, err)

	got := make(map[string]string, len(rows))
	for _, r := range rows {
		got[r.ID] = r.TenantID
	}
	require.Equal(t, DefaultTestTenantID, got[staleA], "stale pending in tenant 1 returned")
	require.Equal(t, t2, got[staleB], "stale pending in tenant 2 returned (cross-tenant enumeration)")
	require.NotContains(t, got, freshPending, "orders newer than the cutoff are excluded")
	require.NotContains(t, got, paidOld, "non-pending orders are excluded")
}

// TestDiscardPendingOrderHandler covers the POS discard handler: invalid id → 400,
// missing order → idempotent 204, non-POS order → 403, POS pending → 204 + deleted.
func TestDiscardPendingOrderHandler(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	cfg := TestConfig()

	createOrder := func(origin order.Origin) string {
		ctx, client, cleanup := privRequestTx(t, tdb, DefaultTestTenantID)
		defer cleanup()
		ord, err := NewRepositories(client).Order.Create(ctx, 100, order.StatusPending, origin, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		return ord.ID
	}
	discard := func(orderID string) int {
		ctx, client, cleanup := privRequestTx(t, tdb, DefaultTestTenantID)
		defer cleanup()
		repos := NewRepositories(client)
		orders := service.NewOrderService(repos.Order, repos.OrderLine, repos.Inventory, nil)
		h := api.NewOrderTestHandlers(orders, NewPaymentSvc(cfg, repos, NewProductSvc(repos)), zap.NewNop())
		rec := httptest.NewRecorder()
		h.DiscardPendingOrder(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx), orderID)
		return rec.Code
	}
	exists := func(orderID string) bool {
		ctx, client, cleanup := privRequestTx(t, tdb, DefaultTestTenantID)
		defer cleanup()
		_, err := NewRepositories(client).Order.GetByID(ctx, orderID)
		return err == nil
	}

	require.Equal(t, http.StatusBadRequest, discard("not a valid id"))
	require.Equal(t, http.StatusNoContent, discard(nanoid.New()), "missing order is an idempotent no-op")

	shopOrder := createOrder(order.OriginShop)
	require.Equal(t, http.StatusForbidden, discard(shopOrder), "shop orders can't be discarded here")
	require.True(t, exists(shopOrder), "a forbidden discard leaves the order intact")

	posOrder := createOrder(order.OriginPos)
	require.Equal(t, http.StatusNoContent, discard(posOrder))
	require.False(t, exists(posOrder), "a discarded POS order is deleted")
}
