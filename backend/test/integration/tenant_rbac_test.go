package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/auth"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// seedUserWithEmail inserts a user carrying an email and returns its id.
func (e *rlsEnv) seedUserWithEmail(t *testing.T, email string) string {
	t.Helper()
	uid := "usr_" + randSuffix()
	e.privExec(t, `INSERT INTO "user" (id, email) VALUES ($1, $2)`, uid, email)
	return uid
}

// seedSession inserts a session row for a user and returns its id.
func (e *rlsEnv) seedSession(t *testing.T, userID string) string {
	t.Helper()
	sid := "sess_" + randSuffix()
	e.privExec(t,
		`INSERT INTO "session" (id, token, user_id, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, now() + interval '90 days', now(), now())`,
		sid, "tok_"+randSuffix(), userID,
	)
	return sid
}

// callTenantRole drives RequireTenantRole over a request tx bound to the given
// tenant on the app pool, with the supplied auth context, and returns the HTTP
// status the gate produced (200 means it passed through to the handler).
func (e *rlsEnv) callTenantRole(t *testing.T, tenantID, userID string, isPlatformAdmin bool, min auth.TenantTier) int {
	t.Helper()
	logger := zap.NewNop()
	mw := auth.NewSessionMiddleware(nil, nil, logger)

	baseCtx, _, cleanup := e.appRequestTx(t, tenantID)
	defer cleanup()

	rc := baseCtx
	if userID != "" {
		rc = auth.WithUserID(rc, userID)
	}
	if isPlatformAdmin {
		rc = auth.WithIsPlatformAdmin(rc, true)
	}

	final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/settings", nil).WithContext(rc)
	rec := httptest.NewRecorder()
	mw.RequireTenantRole(min)(final).ServeHTTP(rec, req)
	return rec.Code
}

// TestRequireTenantRole proves the membership-driven /admin gate: tier checks,
// the closed cross-tenant gap, the platform-admin bypass, and the unlinked-tenant
// lockout that the backfill exists to prevent.
func TestRequireTenantRole(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "rbac-a-"+randSuffix())
	tenantB := env.seedTenant(t, "rbac-b-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "rbac-a-"+randSuffix())
	orgB := env.seedOrgForTenant(t, tenantB, "rbac-b-"+randSuffix())

	owner := env.seedUser(t)
	ops := env.seedUser(t)
	stranger := env.seedUser(t)
	env.seedMember(t, orgA, owner, "owner")
	env.seedMember(t, orgA, ops, "ops")
	// owner is a member of org B too, to exercise the cross-tenant case from a
	// real membership rather than a no-membership 403.
	ownerOfB := env.seedUser(t)
	env.seedMember(t, orgB, ownerOfB, "owner")

	t.Run("tenant-admin role passes the tenant-admin tier", func(t *testing.T) {
		require.Equal(t, http.StatusOK, env.callTenantRole(t, tenantA, owner, false, auth.TierTenantAdmin))
	})

	t.Run("tenant-admin role passes the ops tier", func(t *testing.T) {
		require.Equal(t, http.StatusOK, env.callTenantRole(t, tenantA, owner, false, auth.TierOps))
	})

	t.Run("ops role passes the ops tier", func(t *testing.T) {
		require.Equal(t, http.StatusOK, env.callTenantRole(t, tenantA, ops, false, auth.TierOps))
	})

	t.Run("ops role is blocked from the tenant-admin tier", func(t *testing.T) {
		require.Equal(t, http.StatusForbidden, env.callTenantRole(t, tenantA, ops, false, auth.TierTenantAdmin))
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		require.Equal(t, http.StatusForbidden, env.callTenantRole(t, tenantA, stranger, false, auth.TierOps))
	})

	t.Run("cross-tenant member is forbidden", func(t *testing.T) {
		// ownerOfB is an owner of org B but the request is bound to tenant A, so the
		// org-keyed member lookup returns nothing and the gate refuses.
		require.Equal(t, http.StatusForbidden, env.callTenantRole(t, tenantA, ownerOfB, false, auth.TierOps))
	})

	t.Run("platform admin bypasses", func(t *testing.T) {
		require.Equal(t, http.StatusOK, env.callTenantRole(t, tenantA, stranger, true, auth.TierTenantAdmin))
	})

	t.Run("unlinked tenant locks members out", func(t *testing.T) {
		// A tenant with no organization has app_current_org_id() = NULL, so even a
		// would-be member matches nothing. This is why the backfill links every
		// pre-existing tenant before the gate ships.
		unlinked := env.seedTenant(t, "rbac-unlinked-"+randSuffix())
		require.Equal(t, http.StatusForbidden, env.callTenantRole(t, unlinked, owner, false, auth.TierOps))
	})
}

// seedUserWithRole inserts a user carrying a global role (e.g. 'admin') and an
// email, returning its id. Used to prove the global role grants nothing
// cross-tenant for membership-driven data routes.
func (e *rlsEnv) seedUserWithRole(t *testing.T, role string) string {
	t.Helper()
	uid := "usr_" + randSuffix()
	e.privExec(t, `INSERT INTO "user" (id, email, role) VALUES ($1, $2, $3)`,
		uid, uid+"@example.test", role)
	return uid
}

// callHasTenantTier evaluates auth.HasTenantTier over a request tx bound to the
// tenant, mirroring the /orders admin-branch decision.
func (e *rlsEnv) callHasTenantTier(t *testing.T, tenantID, userID string, isPlatformAdmin bool, min auth.TenantTier) bool {
	t.Helper()
	baseCtx, _, cleanup := e.appRequestTx(t, tenantID)
	defer cleanup()

	rc := baseCtx
	if userID != "" {
		rc = auth.WithUserID(rc, userID)
	}
	if isPlatformAdmin {
		rc = auth.WithIsPlatformAdmin(rc, true)
	}

	ok, err := auth.HasTenantTier(rc, min)
	require.NoError(t, err)
	return ok
}

// TestHasTenantTierOrdersGate proves the /orders admin-listing branch is
// membership-driven: a tenant member (ops/owner) of the bound tenant — or a
// platform admin — gets the full listing, while a user who is NOT a member of the
// bound tenant gets only their own orders even when their global user.role is
// 'admin'. This closes the cross-tenant /orders read.
func TestHasTenantTierOrdersGate(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "ordgate-a-"+randSuffix())
	tenantB := env.seedTenant(t, "ordgate-b-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "ordgate-a-"+randSuffix())
	orgB := env.seedOrgForTenant(t, tenantB, "ordgate-b-"+randSuffix())

	owner := env.seedUser(t)
	ops := env.seedUser(t)
	env.seedMember(t, orgA, owner, "owner")
	env.seedMember(t, orgA, ops, "ops")

	// A global-admin who is a member of tenant B but NOT tenant A. Hitting tenant
	// A's host, the global role must grant nothing.
	globalAdminOfB := env.seedUserWithRole(t, "admin")
	env.seedMember(t, orgB, globalAdminOfB, "admin")

	// A global-admin with no membership anywhere.
	globalAdminNoMember := env.seedUserWithRole(t, "admin")

	t.Run("owner member gets the admin listing", func(t *testing.T) {
		require.True(t, env.callHasTenantTier(t, tenantA, owner, false, auth.TierOps))
	})

	t.Run("ops member gets the admin listing", func(t *testing.T) {
		require.True(t, env.callHasTenantTier(t, tenantA, ops, false, auth.TierOps))
	})

	t.Run("global admin member of another tenant gets only own orders", func(t *testing.T) {
		require.False(t, env.callHasTenantTier(t, tenantA, globalAdminOfB, false, auth.TierOps))
	})

	t.Run("global admin with no membership gets only own orders", func(t *testing.T) {
		require.False(t, env.callHasTenantTier(t, tenantA, globalAdminNoMember, false, auth.TierOps))
	})

	t.Run("platform admin gets the admin listing", func(t *testing.T) {
		require.True(t, env.callHasTenantTier(t, tenantA, globalAdminNoMember, true, auth.TierOps))
	})
}

// TestHasTenantTierMissingTx asserts the helper fails closed (error) when the
// request tx is absent, so callers surface 500 rather than leak.
func TestHasTenantTierMissingTx(t *testing.T) {
	_ = newRLSEnv(t) // gate on the test DB env being present
	ctx := auth.WithUserID(context.Background(), "usr_x")
	_, err := auth.HasTenantTier(ctx, auth.TierOps)
	require.Error(t, err)
}

// TestRequireTenantRoleMissingTx asserts the gate fails closed (500) when the
// request tx is absent, rather than silently allowing the request.
func TestRequireTenantRoleMissingTx(t *testing.T) {
	_ = newRLSEnv(t) // gate on the test DB env being present
	mw := auth.NewSessionMiddleware(nil, nil, zap.NewNop())

	ctx := auth.WithUserID(context.Background(), "usr_x")
	final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/v1/settings", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mw.RequireTenantRole(auth.TierOps)(final).ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
