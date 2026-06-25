package integration

import (
	"testing"
	"time"

	"backend/internal/generated/ent/admininvite"
	nanoid "backend/internal/id"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

// seedAdminInvite inserts an accepted-flow staff invite for a tenant via the
// privileged pool and returns its plaintext token (the hash is derived here so
// the test can present the token to Accept).
func (e *rlsEnv) seedAdminInvite(t *testing.T, tenantID, inviterID, email, memberRole, token string) {
	t.Helper()
	e.privExec(t,
		`INSERT INTO "admin_invite" (id, tenant_id, invited_by_user_id, invitee_email, token_hash, status, member_role, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, now(), now())`,
		nanoid.New(), tenantID, inviterID, email, repository.HashToken(token), memberRole,
		time.Now().Add(24*time.Hour),
	)
}

// acceptStaffInvite runs AdminInviteService.Accept through a request tx bound to
// the tenant and commits it, so privileged reads afterwards see the result. This
// reproduces the production path: Accept's member insert runs on the request tx
// via app_current_org_id().
func (e *rlsEnv) acceptStaffInvite(t *testing.T, tenantID, token string) {
	t.Helper()
	ctx, client, cleanup := e.appRequestTx(t, tenantID)
	svc := service.NewAdminInviteService(
		TestConfig(),
		repository.NewAdminInviteRepository(client),
		repository.NewUserRepository(client),
		noopEmail{},
	)
	require.NoError(t, svc.Accept(ctx, token, "Staff", "Member"))
	cleanup() // commits the request tx
}

// TestStaffInviteCreatesMember proves accepting a staff invite records the user
// as a tenant-scoped member at the invite's tier, via the request tx's
// app_current_org_id().
func TestStaffInviteCreatesMember(t *testing.T) {
	env := newRLSEnv(t)

	cases := []struct {
		name       string
		memberRole string
	}{
		{"catalog tier", "catalog"},
		{"staff tier", "staff"},
		{"admin tier", "admin"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tenantID := env.seedTenant(t, "staff-"+randSuffix())
			orgID := env.seedOrgForTenant(t, tenantID, "staff-"+randSuffix())
			inviter := env.seedUser(t)

			email := "staff-" + randSuffix() + "@example.test"
			token := "staff-token-" + randSuffix()
			env.seedAdminInvite(t, tenantID, inviter, email, tc.memberRole, token)

			env.acceptStaffInvite(t, tenantID, token)

			var role string
			require.NoError(t, env.priv.QueryRow(
				`SELECT m.role FROM "member" m JOIN "user" u ON u.id = m.user_id
				 WHERE m.organization_id = $1 AND lower(u.email) = lower($2)`,
				orgID, email).Scan(&role))
			require.Equal(t, tc.memberRole, role)
		})
	}
}

// TestStaffInviteReacceptUpgrades proves re-accepting with a higher tier upgrades
// the existing membership rather than failing on the unique (org,user) index.
func TestStaffInviteReacceptUpgrades(t *testing.T) {
	env := newRLSEnv(t)

	tenantID := env.seedTenant(t, "staff-up-"+randSuffix())
	orgID := env.seedOrgForTenant(t, tenantID, "staff-up-"+randSuffix())
	inviter := env.seedUser(t)
	email := "upgrade-" + randSuffix() + "@example.test"

	accept := func(memberRole string) {
		token := "tok-" + randSuffix()
		env.seedAdminInvite(t, tenantID, inviter, email, memberRole, token)
		env.acceptStaffInvite(t, tenantID, token)
	}

	accept("staff")
	accept("admin")

	var role string
	require.NoError(t, env.priv.QueryRow(
		`SELECT m.role FROM "member" m JOIN "user" u ON u.id = m.user_id
		 WHERE m.organization_id = $1 AND lower(u.email) = lower($2)`,
		orgID, email).Scan(&role))
	require.Equal(t, "admin", role)
}

// TestStaffInviteUnlinkedTenantNoMember proves accepting a staff invite for a
// tenant with no linked organization fail-closes: the guarded member insert
// matches no rows (app_current_org_id() is NULL), so Accept errors before the
// invite is marked accepted and the request tx rolls back. No NULL-org member is
// ever written, and the invite stays pending so it can be re-accepted once the
// tenant is linked.
func TestStaffInviteUnlinkedTenantNoMember(t *testing.T) {
	env := newRLSEnv(t)

	tenantID := env.seedTenant(t, "staff-nl-"+randSuffix())
	inviter := env.seedUser(t)
	email := "nolink-" + randSuffix() + "@example.test"
	token := "tok-" + randSuffix()
	env.seedAdminInvite(t, tenantID, inviter, email, "staff", token)

	ctx, client, rollback := env.appRequestTxRollback(t, tenantID)
	svc := service.NewAdminInviteService(
		TestConfig(),
		repository.NewAdminInviteRepository(client),
		repository.NewUserRepository(client),
		noopEmail{},
	)
	err := svc.Accept(ctx, token, "Staff", "Member")
	rollback()
	require.Error(t, err)

	var n int
	require.NoError(t, env.priv.QueryRow(
		`SELECT count(*) FROM "member" m JOIN "user" u ON u.id = m.user_id WHERE lower(u.email) = lower($1)`,
		email).Scan(&n))
	require.Equal(t, 0, n)

	var status admininvite.Status
	require.NoError(t, env.priv.QueryRow(
		`SELECT status FROM "admin_invite" WHERE invitee_email = $1`, email).Scan(&status))
	require.Equal(t, admininvite.StatusPending, status)
}
