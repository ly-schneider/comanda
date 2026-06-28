package integration

import (
	"context"
	"testing"

	"backend/internal/database"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

// TestListManageableTenants proves the account-hub tenant listing (privileged
// pool, RLS-bypassing) only exposes tenants the caller may manage: owner/admin
// memberships for a regular user, every tenant for a platform admin.
func TestListManageableTenants(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "lmt-a-"+randSuffix())
	tenantB := env.seedTenant(t, "lmt-b-"+randSuffix())
	tenantC := env.seedTenant(t, "lmt-c-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "lmt-a-"+randSuffix())
	orgB := env.seedOrgForTenant(t, tenantB, "lmt-b-"+randSuffix())
	env.seedOrgForTenant(t, tenantC, "lmt-c-"+randSuffix())

	ownerA := env.seedUser(t)
	adminA := env.seedUser(t)
	appAdminA := env.seedUser(t)
	staffA := env.seedUser(t)
	multi := env.seedUser(t) // admin of A, owner of B
	platformUser := env.seedUser(t)
	env.seedMember(t, orgA, ownerA, "owner")
	env.seedMember(t, orgA, adminA, "admin")
	env.seedMember(t, orgA, appAdminA, "app_admin")
	env.seedMember(t, orgA, staffA, "staff")
	env.seedMember(t, orgA, multi, "admin")
	env.seedMember(t, orgB, multi, "owner")

	svc := service.NewAccountService(&database.PrivilegedClient{Client: env.privClient(), DB: env.priv})
	ctx := context.Background()

	idSet := func(ts []service.ManageableTenant) map[string]bool {
		m := make(map[string]bool, len(ts))
		for _, x := range ts {
			m[x.ID] = true
		}
		return m
	}

	t.Run("owner sees only their tenant", func(t *testing.T) {
		res, err := svc.ListManageableTenants(ctx, ownerA, false)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, tenantA, res[0].ID)
		require.Equal(t, "owner", res[0].Role)
	})

	t.Run("admin sees only their tenant", func(t *testing.T) {
		res, err := svc.ListManageableTenants(ctx, adminA, false)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, tenantA, res[0].ID)
	})

	t.Run("app_admin sees none (below the settings tier)", func(t *testing.T) {
		res, err := svc.ListManageableTenants(ctx, appAdminA, false)
		require.NoError(t, err)
		require.Empty(t, res)
	})

	t.Run("staff sees none", func(t *testing.T) {
		res, err := svc.ListManageableTenants(ctx, staffA, false)
		require.NoError(t, err)
		require.Empty(t, res)
	})

	t.Run("multi-org owner/admin sees both", func(t *testing.T) {
		res, err := svc.ListManageableTenants(ctx, multi, false)
		require.NoError(t, err)
		set := idSet(res)
		require.Len(t, res, 2)
		require.True(t, set[tenantA] && set[tenantB])
	})

	t.Run("platform admin sees all tenants", func(t *testing.T) {
		res, err := svc.ListManageableTenants(ctx, platformUser, true)
		require.NoError(t, err)
		set := idSet(res)
		require.True(t, set[tenantA] && set[tenantB] && set[tenantC])
	})
}
