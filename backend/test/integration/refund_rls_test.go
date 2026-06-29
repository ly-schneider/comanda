package integration

import (
	"testing"

	pgRepo "backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// TestRefundRLS_CrossTenantInvisible proves a refund (and its lines) seeded for
// tenant A is invisible to tenant B through the RLS-bound app role on the repo
// surface, while tenant A reads it back.
func TestRefundRLS_CrossTenantInvisible(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "refund-rls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "refund-rls-b-"+randSuffix())

	orderA := nanoid.New()
	env.privExec(t, `INSERT INTO "order" (id, total_cents, status, origin, tenant_id) VALUES ($1, 1000, 'paid', 'pos', $2)`, orderA, tenantA)

	refundA := nanoid.New()
	env.privExec(t,
		`INSERT INTO refund (id, tenant_id, order_id, amount_cents, kind, method, actor_type, actor_id, status)
		 VALUES ($1, $2, $3, 500, 'arbitrary', 'cash', 'admin', 'admin_1', 'succeeded')`,
		refundA, tenantA, orderA,
	)

	// A refund_line so SumSucceededLineQtyByOrder's refund_line -> refund join
	// (HasRefundWith) is exercised under RLS. Needs the order_line FK chain.
	catA := nanoid.New()
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'A', $2)`, catA, tenantA)
	prodA := nanoid.New()
	env.privExec(t, `INSERT INTO product (id, category_id, name, tenant_id) VALUES ($1, $2, 'P', $3)`, prodA, catA, tenantA)
	lineA := nanoid.New()
	env.privExec(t,
		`INSERT INTO order_line (id, order_id, line_type, product_id, title, quantity, unit_price_cents, tenant_id)
		 VALUES ($1, $2, 'simple', $3, 'P', 2, 350, $4)`,
		lineA, orderA, prodA, tenantA,
	)
	env.privExec(t,
		`INSERT INTO refund_line (id, tenant_id, refund_id, order_line_id, quantity, amount_cents)
		 VALUES ($1, $2, $3, $4, 1, 350)`,
		nanoid.New(), tenantA, refundA, lineA,
	)

	t.Run("tenant A reads its own refund", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()
		repo := pgRepo.NewRefundRepository(client)

		list, err := repo.ListByOrder(ctx, orderA)
		require.NoError(t, err)
		require.Len(t, list, 1)

		sum, err := repo.SumSucceededByOrder(ctx, orderA)
		require.NoError(t, err)
		require.Equal(t, int64(500), sum)

		qty, err := repo.SumSucceededLineQtyByOrder(ctx, orderA)
		require.NoError(t, err)
		require.Equal(t, 1, qty[lineA])
	})

	t.Run("tenant B sees nothing", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()
		repo := pgRepo.NewRefundRepository(client)

		list, err := repo.ListByOrder(ctx, orderA)
		require.NoError(t, err)
		require.Empty(t, list, "tenant B must not see tenant A's refund")

		sum, err := repo.SumSucceededByOrder(ctx, orderA)
		require.NoError(t, err)
		require.Equal(t, int64(0), sum, "tenant B must total zero for tenant A's order")

		qty, err := repo.SumSucceededLineQtyByOrder(ctx, orderA)
		require.NoError(t, err)
		require.Empty(t, qty, "tenant B must not see tenant A's refund_line via the join")
	})
}
