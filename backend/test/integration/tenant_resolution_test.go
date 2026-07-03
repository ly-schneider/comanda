package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/category"
	"backend/internal/middleware"
	"backend/internal/repository"

	nanoid "backend/internal/id"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// privClient builds a privileged ent.Client over the RLS-bypassing pool, used
// by the resolver to read tenant/tenant_domain before any GUC is set.
func (e *rlsEnv) privClient() *ent.Client {
	return ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, e.priv)))
}

// seedDomain registers a hostname -> tenant mapping via the privileged pool.
func (e *rlsEnv) seedDomain(t *testing.T, tenantID string, hostname string) {
	t.Helper()
	// verified_at must be set: the resolver only routes verified rows (an
	// unverified BYO host must not attract traffic), so an unverified seed would
	// never resolve.
	e.privExec(t,
		`INSERT INTO tenant_domain (id, tenant_id, hostname, verified_at) VALUES ($1, $2, $3, now())`,
		nanoid.New(), tenantID, hostname,
	)
}

// seedUnverifiedDomain registers a not-yet-verified BYO hostname (verified_at
// NULL, with a verification token) via the privileged pool.
func (e *rlsEnv) seedUnverifiedDomain(t *testing.T, tenantID string, hostname string) {
	t.Helper()
	e.privExec(t,
		`INSERT INTO tenant_domain (id, tenant_id, hostname, verification_token) VALUES ($1, $2, $3, $4)`,
		nanoid.New(), tenantID, hostname, "tok-"+randSuffix(),
	)
}

// categoryHandler reads category names through the request-scoped Ent client
// (bound by TenantMiddleware.Tx) and writes them comma-joined. It is the probe
// for what a request actually sees under RLS.
func categoryHandler(w http.ResponseWriter, r *http.Request) {
	client := repository.ClientFromContext(r.Context(), nil)
	if client == nil {
		http.Error(w, "no client", http.StatusInternalServerError)
		return
	}
	cats, err := client.Category.Query().Order(ent.Asc(category.FieldName)).All(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := ""
	for i, c := range cats {
		if i > 0 {
			out += ","
		}
		out += c.Name
	}
	_, _ = io.WriteString(w, out)
}

// buildResolverStack wires Resolve -> Tx -> categoryHandler against the app pool
// using the given platform hosts + header-trusted flag.
func (e *rlsEnv) buildResolverStack(platformHosts []string, headerTrusted bool) http.Handler {
	mw := middleware.NewTenantMiddleware(
		e.app, e.privClient(), e.priv, "", platformHosts, headerTrusted, zap.NewNop(),
	)
	h := http.HandlerFunc(categoryHandler)
	return mw.Resolve(mw.Tx(h))
}

func doGet(t *testing.T, h http.Handler, host string, headers map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/categories", nil)
	if host != "" {
		req.Host = host
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, string(body)
}

// TestHostResolutionIsolation: two hostnames map to two tenants; each request
// sees only its own tenant's rows. The core P2 acceptance criterion.
func TestHostResolutionIsolation(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "host-a-"+randSuffix())
	tenantB := env.seedTenant(t, "host-b-"+randSuffix())
	hostA := "a-" + randSuffix() + ".comanda.test"
	hostB := "b-" + randSuffix() + ".comanda.test"
	env.seedDomain(t, tenantA, hostA)
	env.seedDomain(t, tenantB, hostB)

	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'OnlyA', $2)`, nanoid.New(), tenantA)
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'OnlyB', $2)`, nanoid.New(), tenantB)

	h := env.buildResolverStack(nil, false)

	t.Run("X-Forwarded-Host hostA sees only A", func(t *testing.T) {
		code, body := doGet(t, h, "ignored.example", map[string]string{"X-Forwarded-Host": hostA})
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "OnlyA", body)
	})

	t.Run("X-Forwarded-Host hostB sees only B", func(t *testing.T) {
		code, body := doGet(t, h, "ignored.example", map[string]string{"X-Forwarded-Host": hostB})
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "OnlyB", body)
	})

	t.Run("r.Host fallback (no X-Forwarded-Host) resolves hostA with port", func(t *testing.T) {
		code, body := doGet(t, h, hostA+":8443", nil)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "OnlyA", body)
	})

	t.Run("X-Forwarded-Host comma list takes first", func(t *testing.T) {
		code, body := doGet(t, h, "ignored.example", map[string]string{"X-Forwarded-Host": hostB + ", proxy.internal"})
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "OnlyB", body)
	})
}

// TestUnknownHost404: a host with no tenant_domain and no platform/bootstrap
// fallback is rejected with 404 before any tx opens.
func TestUnknownHost404(t *testing.T) {
	env := newRLSEnv(t)
	h := env.buildResolverStack(nil, false)
	code, _ := doGet(t, h, "nobody."+randSuffix()+".test", nil)
	require.Equal(t, http.StatusNotFound, code)
}

// TestUnverifiedDomainDoesNotResolve is the regression test for the resolver's
// verified-only rule: an unverified BYO hostname must not route to its tenant
// (otherwise a tenant could claim a host it does not control), while the same
// tenant's verified hostname does.
func TestUnverifiedDomainDoesNotResolve(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "unv-a-"+randSuffix())
	verifiedHost := "ok-" + randSuffix() + ".comanda.test"
	unverifiedHost := "pending-" + randSuffix() + ".comanda.test"
	env.seedDomain(t, tenantA, verifiedHost)
	env.seedUnverifiedDomain(t, tenantA, unverifiedHost)

	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'OnlyA', $2)`, nanoid.New(), tenantA)

	h := env.buildResolverStack(nil, false)

	t.Run("verified host resolves", func(t *testing.T) {
		code, body := doGet(t, h, verifiedHost, nil)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "OnlyA", body)
	})

	t.Run("unverified host is rejected", func(t *testing.T) {
		code, _ := doGet(t, h, unverifiedHost, nil)
		require.Equal(t, http.StatusNotFound, code)
	})
}

// TestTenantHeaderTrust: X-Tenant-Id is honored only when headerTrusted is true.
func TestTenantHeaderTrust(t *testing.T) {
	env := newRLSEnv(t)
	tenantA := env.seedTenant(t, "hdr-a-"+randSuffix())
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'HdrA', $2)`, nanoid.New(), tenantA)

	unknownHost := "unknown-" + randSuffix() + ".test"

	t.Run("trusted=false ignores X-Tenant-Id -> 404 on unknown host", func(t *testing.T) {
		h := env.buildResolverStack(nil, false)
		code, _ := doGet(t, h, unknownHost, map[string]string{middleware.TenantHeader: tenantA})
		require.Equal(t, http.StatusNotFound, code, "header must be ignored when not trusted")
	})

	t.Run("trusted=true honors X-Tenant-Id", func(t *testing.T) {
		h := env.buildResolverStack(nil, true)
		code, body := doGet(t, h, unknownHost, map[string]string{middleware.TenantHeader: tenantA})
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "HdrA", body)
	})
}

// TestPlatformHostContext: a configured platform host resolves to a no-tenant
// platform context. A platform-admin session reads across tenants; a non-admin
// platform request sees zero rows (RLS fail-closed). Auth is simulated by
// setting the is_platform_admin GUC inside the handler, mirroring what the
// session middleware does after a verified platform-admin session.
func TestPlatformHostContext(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "plat-a-"+randSuffix())
	tenantB := env.seedTenant(t, "plat-b-"+randSuffix())
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'PlatA', $2)`, nanoid.New(), tenantA)
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'PlatB', $2)`, nanoid.New(), tenantB)

	platformHost := "admin-" + randSuffix() + ".comanda.test"

	// adminGUCHandler flips the is_platform_admin GUC on the request tx (as the
	// auth middleware would for a verified platform admin) then reads categories.
	adminGUCHandler := func(isAdmin bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if isAdmin {
				tx, ok := repository.RequestTxFromContext(r.Context())
				require.True(t, ok, "request tx must be in context")
				_, err := tx.ExecContext(r.Context(), "SELECT set_config('app.is_platform_admin', 'true', true)")
				require.NoError(t, err)
			}
			categoryHandler(w, r)
		}
	}

	stack := func(isAdmin bool) http.Handler {
		mw := middleware.NewTenantMiddleware(env.app, env.privClient(), env.priv, "", []string{platformHost}, false, zap.NewNop())
		return mw.Resolve(mw.Tx(adminGUCHandler(isAdmin)))
	}

	t.Run("platform admin reads across tenants", func(t *testing.T) {
		code, body := doGet(t, stack(true), platformHost, nil)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "PlatA")
		require.Contains(t, body, "PlatB")
	})

	t.Run("non-admin platform request sees zero rows", func(t *testing.T) {
		code, body := doGet(t, stack(false), platformHost, nil)
		require.Equal(t, http.StatusOK, code)
		require.Empty(t, body, "non-admin platform context must read no rows")
	})
}

// TestSuspendedTenantBlocked: a suspended tenant's hosts serve nothing — every
// request is rejected 403 at resolution, before any tx opens. Reactivating
// restores traffic.
func TestSuspendedTenantBlocked(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "susp-a-"+randSuffix())
	host := "susp-" + randSuffix() + ".comanda.test"
	env.seedDomain(t, tenantA, host)
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'SuspA', $2)`, nanoid.New(), tenantA)

	h := env.buildResolverStack(nil, true)

	t.Run("active tenant serves traffic", func(t *testing.T) {
		code, body := doGet(t, h, host, nil)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "SuspA", body)
	})

	env.privExec(t, `UPDATE tenant SET status = 'suspended' WHERE id = $1`, tenantA)

	t.Run("suspended tenant host is 403", func(t *testing.T) {
		code, body := doGet(t, h, host, nil)
		require.Equal(t, http.StatusForbidden, code)
		require.Contains(t, body, "tenant_suspended")
	})

	t.Run("suspended tenant via trusted header is 403", func(t *testing.T) {
		code, body := doGet(t, h, "unknown-"+randSuffix()+".test", map[string]string{middleware.TenantHeader: tenantA})
		require.Equal(t, http.StatusForbidden, code)
		require.Contains(t, body, "tenant_suspended")
	})

	env.privExec(t, `UPDATE tenant SET status = 'active' WHERE id = $1`, tenantA)

	t.Run("reactivated tenant serves traffic again", func(t *testing.T) {
		code, body := doGet(t, h, host, nil)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "SuspA", body)
	})
}
