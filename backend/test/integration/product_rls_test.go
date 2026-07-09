package integration

import (
	"testing"

	"backend/internal/generated/ent/product"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// TestProductService_RequestScopedClient is a regression test for the bug where
// ProductRepository queried r.client directly: wired with the static app-pool
// client (as in production), reads must be tenant-scoped and creates must pass
// RLS via the request tx. The shared service instance across both tenants also
// proves the catalog cache is tenant-keyed.
func TestProductService_RequestScopedClient(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "prod-rls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "prod-rls-b-"+randSuffix())

	catA, catB := nanoid.New(), nanoid.New()
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'CatA', $2)`, catA, tenantA)
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'CatB', $2)`, catB, tenantB)

	prodA, prodB := nanoid.New(), nanoid.New()
	env.privExec(t, `INSERT INTO product (id, category_id, name, tenant_id) VALUES ($1, $2, 'OnlyA', $3)`, prodA, catA, tenantA)
	env.privExec(t, `INSERT INTO product (id, category_id, name, tenant_id) VALUES ($1, $2, 'OnlyB', $3)`, prodB, catB, tenantB)

	staticClient := env.appEntClient()
	repos := NewRepositories(staticClient)
	svc := service.NewProductService(
		repos.Product,
		repos.ProductVariant,
		repos.Category,
		repos.BundleSlot,
		repos.BundleSlotOption,
		repos.Inventory,
		repos.Jeton,
		nil,
	)

	t.Run("tenant A request tx sees only its product", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()

		all, err := svc.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
		require.Equal(t, prodA, all[0].ID)

		got, err := svc.GetByID(ctx, prodA)
		require.NoError(t, err)
		require.Equal(t, "OnlyA", got.Name)
	})

	t.Run("tenant B is not served tenant A's cached catalog", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()

		all, err := svc.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1, "catalog cache must be tenant-keyed")
		require.Equal(t, prodB, all[0].ID)
	})

	t.Run("tenant A create passes RLS on the request tx", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()

		created, err := svc.Create(ctx, catA, product.TypeSimple, "NewA", 500, 0, true, nil, nil, nil, nil, nil)
		require.NoError(t, err, "create must run on the GUC-bound request tx, not the raw app pool")
		require.Equal(t, tenantA, created.TenantID)

		all, err := svc.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, all, 2)
	})

	t.Run("tenant B still sees only its product after A's create", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()

		all, err := svc.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
		require.Equal(t, prodB, all[0].ID)
	})

	t.Run("variants are tenant-isolated", func(t *testing.T) {
		variantA := nanoid.New()
		env.privExec(t, `INSERT INTO product_variant (id, product_id, value, tenant_id) VALUES ($1, $2, 'M', $3)`, variantA, prodA, tenantA)

		ctxA, _, cleanupA := env.appRequestTx(t, tenantA)
		defer cleanupA()
		mine, err := repos.ProductVariant.GetByProductID(ctxA, prodA)
		require.NoError(t, err)
		require.Len(t, mine, 1)
		require.Equal(t, variantA, mine[0].ID)

		ctxB, _, cleanupB := env.appRequestTx(t, tenantB)
		defer cleanupB()
		leaked, err := repos.ProductVariant.GetByProductID(ctxB, prodA)
		require.NoError(t, err)
		require.Empty(t, leaked, "tenant B must not see tenant A's variants")

		_, err = repos.ProductVariant.GetByID(ctxB, variantA)
		require.Error(t, err, "direct variant lookup across tenants must fail")
	})
}
