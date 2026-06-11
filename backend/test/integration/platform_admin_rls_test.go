package integration

import (
	"database/sql"
	"testing"

	"backend/internal/enttenant"
	"backend/internal/generated/ent/category"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// TestPlatformAdminRLS proves the two-gate platform-admin design at the DB layer:
//   - is_platform_admin=true + no tenant  -> sees every tenant's rows (bypass).
//   - is_platform_admin=false + no tenant -> sees zero rows (fail-closed).
//
// This is the SQL half; the Go interceptor half is covered separately below.
func TestPlatformAdminRLS(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "pa-a-"+randSuffix())
	tenantB := env.seedTenant(t, "pa-b-"+randSuffix())

	catA := nanoid.New()
	catB := nanoid.New()
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'PA-A', $2)`, catA, tenantA)
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'PA-B', $2)`, catB, tenantB)

	t.Run("platform admin reads across tenants", func(t *testing.T) {
		env.withPlatform(t, true, func(tx *sql.Tx) {
			require.Equal(t, 1, count(t, tx, `SELECT count(*) FROM category WHERE id = $1`, catA))
			require.Equal(t, 1, count(t, tx, `SELECT count(*) FROM category WHERE id = $1`, catB))
		})
	})

	t.Run("non-admin no-tenant sees nothing", func(t *testing.T) {
		env.withPlatform(t, false, func(tx *sql.Tx) {
			require.Equal(t, 0, count(t, tx, `SELECT count(*) FROM category WHERE id IN ($1, $2)`, catA, catB))
		})
	})
}

// TestPlatformAdminEntInterceptor proves the enttenant platform bypass is itself
// safe: in a platform ctx the interceptor adds no tenant predicate, but RLS +
// the GUC remain the boundary. With is_platform_admin=true an Ent query reads
// cross-tenant rows; with it false the same Ent query (still no predicate) reads
// zero rows because RLS denies them.
func TestPlatformAdminEntInterceptor(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "pe-a-"+randSuffix())
	tenantB := env.seedTenant(t, "pe-b-"+randSuffix())
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'PE-A', $2)`, nanoid.New(), tenantA)
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'PE-B', $2)`, nanoid.New(), tenantB)

	t.Run("platform admin Ent query reads both tenants", func(t *testing.T) {
		ctx, client, cleanup := env.appPlatformTx(t, true)
		defer cleanup()

		n, err := client.Category.Query().
			Where(category.NameIn("PE-A", "PE-B")).
			Count(ctx)
		require.NoError(t, err)
		require.Equal(t, 2, n, "platform admin must read across tenants via the Ent client")
	})

	t.Run("non-admin platform Ent query reads zero (RLS denies)", func(t *testing.T) {
		ctx, client, cleanup := env.appPlatformTx(t, false)
		defer cleanup()

		// No predicate is injected (platform ctx), but RLS with is_platform_admin
		// false and tenant NULL returns nothing — fail-closed at the DB layer.
		n, err := client.Category.Query().
			Where(category.NameIn("PE-A", "PE-B")).
			Count(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, n, "non-admin platform ctx must read zero rows")
	})
}

// TestEntFailClosedNoTenant guards the ordinary no-tenant path: a scoped Ent
// query with neither a tenant nor the platform marker must error (ErrNoTenant),
// never silently run unfiltered.
func TestEntFailClosedNoTenant(t *testing.T) {
	env := newRLSEnv(t)

	// Bind an Ent client to an app-pool tx with NO tenant ctx and NO platform
	// marker — the P1 fail-closed contract.
	ctx, client, cleanup := env.appRequestTxNoTenant(t)
	defer cleanup()

	_, err := client.Category.Query().Count(ctx)
	require.ErrorIs(t, err, enttenant.ErrNoTenant)
}
