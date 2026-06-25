package integration

import (
	"testing"

	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

// appEntClient builds a static (non-tx) Ent client over the RLS-bound app pool,
// mirroring the FX-wired database.NewAppClient: the tenant guards are registered
// but no app.tenant_id GUC is bound to its pooled connections. This is exactly
// the client that services receive as their `client` field, so a service method
// that queries it directly (instead of repository.ClientFromContext) runs with
// no GUC and RLS denies every row.
func (e *rlsEnv) appEntClient() *ent.Client {
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, e.app)))
	enttenant.Register(client)
	return client
}

// TestStationService_RequestScopedClient is a regression test for the bug where
// stationService queried s.client.Device directly. The service is wired with the
// STATIC app-pool client (as in production), but called inside a per-tenant
// request tx. Only by routing through repository.ClientFromContext(ctx, s.client)
// do the reads/writes land on the GUC-bound tx connection; querying the static
// client runs with no app.tenant_id and RLS returns zero rows.
func TestStationService_RequestScopedClient(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "station-rls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "station-rls-b-"+randSuffix())

	stationA := nanoid.New()
	env.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id)
		 VALUES ($1, 'Station A', $2, 'STATION', 'approved', $3)`,
		stationA, "stn-"+randSuffix(), tenantA,
	)

	// Static, FX-style client: NOT tx-scoped. The service must reach the
	// request-scoped client via ctx, not this one. It shares the rlsEnv app pool
	// (closed by newRLSEnv's t.Cleanup), so it is intentionally not closed here.
	staticClient := env.appEntClient()
	repos := NewRepositories(staticClient)
	svc := service.NewStationService(
		TestConfig(), staticClient,
		repos.Device, repos.DeviceProduct, repos.OrderLine, repos.OrderRedemption, repos.Idempotency,
	)

	t.Run("tenant A request tx sees its station", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()

		list, err := svc.ListStations(ctx, nil)
		require.NoError(t, err)
		require.Len(t, list, 1, "ListStations must return tenant A's station via the request-scoped client")
		require.Equal(t, stationA, list[0].ID)

		got, err := svc.GetStationByID(ctx, stationA)
		require.NoError(t, err, "GetStationByID must resolve the station via the request-scoped client")
		require.Equal(t, stationA, got.ID)

		renamed, err := svc.RenameStation(ctx, stationA, "Station A renamed")
		require.NoError(t, err, "RenameStation must update via the request-scoped client")
		require.Equal(t, "Station A renamed", renamed.Name)
	})

	t.Run("a different tenant's request tx sees nothing", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()

		list, err := svc.ListStations(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, list, "tenant B must not see tenant A's station")

		_, err = svc.GetStationByID(ctx, stationA)
		require.Error(t, err, "tenant B must not resolve tenant A's station by id")
	})
}
