package integration

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/enttenant"
	"backend/internal/middleware"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ── seed helpers (privileged pool; persisted) ────────────────────────────────

// seedOrgForTenant inserts an organization and links the tenant to it 1:1.
func (e *rlsEnv) seedOrgForTenant(t *testing.T, tenantID, slug string) string {
	t.Helper()
	orgID := nanoid.New()
	e.privExec(t, `INSERT INTO "organization" (id, name, slug, created_at) VALUES ($1, $2, $3, now())`, orgID, slug, slug)
	e.privExec(t, `UPDATE tenant SET organization_id = $1 WHERE id = $2`, orgID, tenantID)
	return orgID
}

// seedUser inserts a minimal user (all other columns default) and returns its id.
func (e *rlsEnv) seedUser(t *testing.T) string {
	t.Helper()
	uid := "usr_" + randSuffix()
	e.privExec(t, `INSERT INTO "user" (id) VALUES ($1)`, uid)
	return uid
}

// seedMember inserts an org membership.
func (e *rlsEnv) seedMember(t *testing.T, orgID, userID, role string) {
	t.Helper()
	e.privExec(t,
		`INSERT INTO "member" (id, organization_id, user_id, role, created_at) VALUES ($1, $2, $3, $4, now())`,
		"mem_"+randSuffix(), orgID, userID, role,
	)
}

// seedInvitation inserts a pending invitation (inviter must exist).
func (e *rlsEnv) seedInvitation(t *testing.T, orgID, email, inviterID string) {
	t.Helper()
	e.privExec(t,
		`INSERT INTO "invitation" (id, organization_id, email, inviter_id, expires_at, created_at) VALUES ($1, $2, $3, $4, $5, now())`,
		"inv_"+randSuffix(), orgID, email, inviterID, time.Now().Add(24*time.Hour),
	)
}

func rowCount(t *testing.T, tx *sql.Tx, want int, query string, args ...any) {
	t.Helper()
	var n int
	require.NoError(t, tx.QueryRow(query, args...).Scan(&n))
	require.Equalf(t, want, n, "row count for: %s", query)
}

// ── tests ────────────────────────────────────────────────────────────────────

// TestOrgScopedRLS proves the org-plugin tables are isolated to the current
// tenant's organization: a vendor bound to tenant A sees only org A's
// organization/member/invitation rows and cannot read, update, or delete org B's.
func TestOrgScopedRLS(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "orgrls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "orgrls-b-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "orgrls-a-"+randSuffix())
	orgB := env.seedOrgForTenant(t, tenantB, "orgrls-b-"+randSuffix())

	userA := env.seedUser(t)
	userB := env.seedUser(t)
	env.seedMember(t, orgA, userA, "owner")
	env.seedMember(t, orgB, userB, "owner")
	env.seedInvitation(t, orgA, "a@example.test", userA)
	env.seedInvitation(t, orgB, "b@example.test", userB)

	env.withTenant(t, &tenantA, func(tx *sql.Tx) {
		rowCount(t, tx, 1, "SELECT count(*) FROM organization")
		var visibleOrg string
		require.NoError(t, tx.QueryRow("SELECT id FROM organization").Scan(&visibleOrg))
		require.Equal(t, orgA, visibleOrg)

		rowCount(t, tx, 1, "SELECT count(*) FROM member")
		rowCount(t, tx, 1, "SELECT count(*) FROM invitation")

		rowCount(t, tx, 0, "SELECT count(*) FROM organization WHERE id = $1", orgB)
		rowCount(t, tx, 0, "SELECT count(*) FROM member WHERE organization_id = $1", orgB)
		rowCount(t, tx, 0, "SELECT count(*) FROM invitation WHERE organization_id = $1", orgB)

		// Cross-org writes affect zero rows (RLS hides them from the statement).
		res, err := tx.Exec(`UPDATE member SET role = 'admin' WHERE organization_id = $1`, orgB)
		require.NoError(t, err)
		n, _ := res.RowsAffected()
		require.Equal(t, int64(0), n)

		res, err = tx.Exec(`DELETE FROM invitation WHERE organization_id = $1`, orgB)
		require.NoError(t, err)
		n, _ = res.RowsAffected()
		require.Equal(t, int64(0), n)
	})
}

// TestOrgRLSFailClosed proves the org tables fail closed: with no tenant bound,
// app_current_org_id() is NULL and the policies expose nothing.
func TestOrgRLSFailClosed(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "orgfc-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "orgfc-"+randSuffix())
	userA := env.seedUser(t)
	env.seedMember(t, orgA, userA, "owner")
	env.seedInvitation(t, orgA, "fc@example.test", userA)

	env.withTenant(t, nil, func(tx *sql.Tx) {
		rowCount(t, tx, 0, "SELECT count(*) FROM organization")
		rowCount(t, tx, 0, "SELECT count(*) FROM member")
		rowCount(t, tx, 0, "SELECT count(*) FROM invitation")
	})
}

// TestPlatformAdminSeesAllOrgs proves the platform-admin bypass spans every org;
// without it (no bypass GUC, no tenant) the same context sees nothing.
func TestPlatformAdminSeesAllOrgs(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "orgpa-a-"+randSuffix())
	tenantB := env.seedTenant(t, "orgpa-b-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "orgpa-a-"+randSuffix())
	orgB := env.seedOrgForTenant(t, tenantB, "orgpa-b-"+randSuffix())
	userA := env.seedUser(t)
	env.seedMember(t, orgA, userA, "owner")

	env.withPlatform(t, true, func(tx *sql.Tx) {
		rowCount(t, tx, 1, "SELECT count(*) FROM organization WHERE id = $1", orgA)
		rowCount(t, tx, 1, "SELECT count(*) FROM organization WHERE id = $1", orgB)
		rowCount(t, tx, 1, "SELECT count(*) FROM member WHERE organization_id = $1", orgA)
	})

	env.withPlatform(t, false, func(tx *sql.Tx) {
		rowCount(t, tx, 0, "SELECT count(*) FROM organization WHERE id = $1", orgA)
		rowCount(t, tx, 0, "SELECT count(*) FROM member WHERE organization_id = $1", orgA)
	})
}

// TestVendorTenantResolutionQuery pins the exact privileged lookup the
// user→tenant resolver runs: a tenant id comes back only when the user is a
// member of an organization that is linked to a tenant.
func TestVendorTenantResolutionQuery(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "vtr-a-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "vtr-a-"+randSuffix())
	member := env.seedUser(t)
	stranger := env.seedUser(t)
	env.seedMember(t, orgA, member, "owner")

	// Organization with a member but no linked tenant.
	orgUnlinked := nanoid.New()
	env.privExec(t, `INSERT INTO "organization" (id, name, slug, created_at) VALUES ($1, $2, $3, now())`,
		orgUnlinked, "unlinked-"+randSuffix(), "unlinked-"+randSuffix())
	env.seedMember(t, orgUnlinked, member, "owner")

	const q = `
		SELECT t.id
		FROM tenant t
		JOIN member m ON m.organization_id = t.organization_id
		WHERE t.organization_id = $1 AND m.user_id = $2`

	t.Run("member resolves to the linked tenant", func(t *testing.T) {
		var got string
		require.NoError(t, env.priv.QueryRow(q, orgA, member).Scan(&got))
		require.Equal(t, tenantA, got)
	})

	t.Run("non-member is denied", func(t *testing.T) {
		err := env.priv.QueryRow(q, orgA, stranger).Scan(new(string))
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("member of an unlinked org resolves to nothing", func(t *testing.T) {
		err := env.priv.QueryRow(q, orgUnlinked, member).Scan(new(string))
		require.ErrorIs(t, err, sql.ErrNoRows)
	})
}

// TestResolveVendorTenantMiddleware exercises the resolver end to end against the
// app pool: a member is bound to their tenant; everyone else is refused.
func TestResolveVendorTenantMiddleware(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "rvt-a-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "rvt-a-"+randSuffix())
	member := env.seedUser(t)
	stranger := env.seedUser(t)
	env.seedMember(t, orgA, member, "owner")

	mw := middleware.NewTenantMiddleware(env.app, env.privClient(), env.priv, "", nil, false, zap.NewNop())

	// call drives ResolveVendorTenant over a platform-context (or tenant-context)
	// request tx on the app pool, returning the HTTP status and the tenant the
	// downstream handler observed.
	call := func(t *testing.T, platform bool, userID, orgID string) (int, string, bool) {
		t.Helper()
		var baseCtx context.Context
		var cleanup func()
		if platform {
			baseCtx, _, cleanup = env.appPlatformTx(t, false)
		} else {
			baseCtx, _, cleanup = env.appRequestTx(t, tenantA)
		}
		defer cleanup()

		rc := baseCtx
		if userID != "" {
			rc = auth.WithUserID(rc, userID)
		}
		if orgID != "" {
			rc = auth.WithActiveOrgID(rc, orgID)
		}

		var bound string
		var platformAfter bool
		final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id, ok := auth.GetTenantID(r.Context()); ok {
				bound = id
			}
			platformAfter = enttenant.IsPlatform(r.Context())
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/account/tenant", nil).WithContext(rc)
		rec := httptest.NewRecorder()
		mw.ResolveVendorTenant(final).ServeHTTP(rec, req)
		return rec.Code, bound, platformAfter
	}

	t.Run("member is bound to their tenant", func(t *testing.T) {
		code, bound, platformAfter := call(t, true, member, orgA)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, tenantA, bound)
		// The platform marker is cleared once a concrete tenant is bound, so the
		// Ent guard scopes (no platform bypass) downstream.
		require.False(t, platformAfter)
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		code, bound, _ := call(t, true, stranger, orgA)
		require.Equal(t, http.StatusForbidden, code)
		require.Empty(t, bound)
	})

	t.Run("missing active org is forbidden", func(t *testing.T) {
		code, _, _ := call(t, true, member, "")
		require.Equal(t, http.StatusForbidden, code)
	})

	t.Run("non-platform host is rejected", func(t *testing.T) {
		code, _, _ := call(t, false, member, orgA)
		require.Equal(t, http.StatusBadRequest, code)
	})
}
