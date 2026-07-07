package integration

import (
	"database/sql"
	"testing"

	"backend/internal/plugin"
	pgRepo "backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTenantPluginRLS_CrossTenantEnablement(t *testing.T) {
	env := newRLSEnv(t)
	tenantA := env.seedTenant(t, "tp-rls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "tp-rls-b-"+randSuffix())

	env.privExec(t,
		`INSERT INTO tenant_plugin (id, tenant_id, plugin_key, enabled, config) VALUES ($1, $2, 'fixture', true, '{"x":1}')`,
		nanoid.New(), tenantA,
	)

	plugins := []plugin.Plugin{fixturePlugin{}}

	t.Run("tenant A sees its enablement", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()
		svc := plugin.NewService(plugins, pgRepo.NewTenantPluginRepository(client), nil, zap.NewNop())

		enabled, err := svc.Enabled(ctx, "fixture")
		require.NoError(t, err)
		require.True(t, enabled)

		list, err := svc.ListPublic(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, "fixture", list[0].Key)
	})

	t.Run("tenant B cannot see tenant A's enablement", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()
		svc := plugin.NewService(plugins, pgRepo.NewTenantPluginRepository(client), nil, zap.NewNop())

		enabled, err := svc.Enabled(ctx, "fixture")
		require.NoError(t, err)
		require.False(t, enabled)

		list, err := svc.ListPublic(ctx)
		require.NoError(t, err)
		require.Empty(t, list)
	})

	t.Run("row invisible to tenant B at the DB layer", func(t *testing.T) {
		idB := tenantB
		env.withTenant(t, &idB, func(tx *sql.Tx) {
			var count int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM tenant_plugin WHERE plugin_key = 'fixture'`).Scan(&count))
			require.Zero(t, count)
		})
		idA := tenantA
		env.withTenant(t, &idA, func(tx *sql.Tx) {
			var count int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM tenant_plugin WHERE plugin_key = 'fixture'`).Scan(&count))
			require.Equal(t, 1, count)
		})
	})
}
