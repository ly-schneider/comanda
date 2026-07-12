package integration

import (
	"context"
	"testing"
	"time"

	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/inventory"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func TestInventoryStream_TenantIsolation(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "inv-sse-a-"+randSuffix())
	tenantB := env.seedTenant(t, "inv-sse-b-"+randSuffix())

	catA := nanoid.New()
	prodA := nanoid.New()
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'CatA', $2)`, catA, tenantA)
	env.privExec(t, `INSERT INTO product (id, category_id, name, tenant_id) VALUES ($1, $2, 'OnlyA', $3)`, prodA, catA, tenantA)

	hub := inventory.NewHub()
	// The service now publishes via inventory.Publisher (prod: a pg_notify that a
	// per-pod listener turns back into hub.Publish). Bridge straight to the hub
	// here so this test keeps exercising service-publish → hub tenant isolation.
	pub := inventory.PublisherFunc(func(_ context.Context, u inventory.Update) { hub.Publish(u) })
	repos := NewRepositories(env.appEntClient())
	svc := service.NewProductService(
		repos.Product,
		repos.ProductVariant,
		repos.Category,
		repos.BundleSlot,
		repos.BundleSlotOption,
		repos.Inventory,
		repos.Jeton,
		pub,
		service.NewCatalogCache(),
	)

	chA := hub.Subscribe(nanoid.New(), tenantA)
	chB := hub.Subscribe(nanoid.New(), tenantB)

	ctxA, _, cleanup := env.appRequestTx(t, tenantA)
	defer cleanup()

	err := svc.AdjustStock(ctxA, prodA, nil, 5, string(inventoryledger.ReasonManualAdjust))
	require.NoError(t, err)

	select {
	case update := <-chA:
		require.Equal(t, prodA, update.ProductID)
		require.Equal(t, 5, update.NewStock)
	case <-time.After(2 * time.Second):
		t.Fatal("tenant A subscriber received no update for its own stock adjustment")
	}

	select {
	case update := <-chB:
		t.Fatalf("tenant B subscriber leaked tenant A update: %+v", update)
	case <-time.After(200 * time.Millisecond):
	}
}
