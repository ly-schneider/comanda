package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/inventory"
	"backend/internal/repository"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Exercises the real cross-pod path: the notifier emits pg_notify on the request
// tx, and a dedicated LISTEN connection (as each pod runs) receives it on commit.
func TestInventoryNotify_ListenFanout(t *testing.T) {
	env := newRLSEnv(t)

	tenant := env.seedTenant(t, "inv-notify-"+randSuffix())
	cat := nanoid.New()
	prod := nanoid.New()
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'Cat', $2)`, cat, tenant)
	env.privExec(t, `INSERT INTO product (id, category_id, name, tenant_id) VALUES ($1, $2, 'Item', $3)`, prod, cat, tenant)

	listenConn, err := pgx.Connect(context.Background(), os.Getenv("POSTGRES_TEST_DSN_APP"))
	require.NoError(t, err)
	defer func() { _ = listenConn.Close(context.Background()) }()
	_, err = listenConn.Exec(context.Background(), "LISTEN "+inventory.NotifyChannel)
	require.NoError(t, err)

	repos := NewRepositories(env.appEntClient())
	svc := service.NewProductService(
		repos.Product, repos.ProductVariant, repos.Category, repos.BundleSlot,
		repos.BundleSlotOption, repos.Inventory, repos.Jeton,
		repository.NewInventoryNotifier(zap.NewNop()),
		service.NewCatalogCache(),
	)

	ctx, _, commit := env.appRequestTx(t, tenant)
	require.NoError(t, svc.AdjustStock(ctx, prod, nil, 5, string(inventoryledger.ReasonManualAdjust)))
	commit() // NOTIFY is delivered on commit, not before.

	deadline := time.Now().Add(3 * time.Second)
	var update inventory.Update
	for {
		waitCtx, cancel := context.WithDeadline(context.Background(), deadline)
		n, err := listenConn.WaitForNotification(waitCtx)
		cancel()
		require.NoError(t, err, "no NOTIFY for the committed stock change")
		u, err := inventory.ParseUpdate(n.Payload)
		require.NoError(t, err)
		if u.ProductID == prod { // ignore any stray notifications from other tests
			update = u
			break
		}
	}

	require.Equal(t, tenant, update.TenantID)
	require.Equal(t, 5, update.NewStock)
}
