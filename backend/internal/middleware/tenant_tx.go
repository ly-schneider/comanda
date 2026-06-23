package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenant"
	"backend/internal/generated/ent/tenantdomain"
	"backend/internal/repository"
	"backend/internal/response"

	nanoid "backend/internal/id"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// TenantHeader is the internal/test header carrying the tenant UUID. It is only
// honoured when TenantHeaderTrusted is set (never in prod), so a public proxy
// can never inject it. Host-based resolution is the production path.
const TenantHeader = "X-Tenant-Id"

// ForwardedHostHeader is the public host forwarded by the web proxy. The proxy
// strips any client-supplied value and sets the real public host here.
const ForwardedHostHeader = "X-Forwarded-Host"

// TenantMiddleware resolves the request's tenant and runs the request inside a
// single tenant-scoped Postgres transaction.
type TenantMiddleware struct {
	appDB         *sql.DB
	privileged    *ent.Client
	privilegedDB  *sql.DB
	bootstrapSlug string
	platformHosts map[string]struct{}
	headerTrusted bool
	logger        *zap.Logger
}

// NewTenantMiddleware builds the middleware. appDB is the RLS-bound app pool;
// privileged resolves the tenant/tenant_domain tables before any tenant is set.
// privilegedDB is that owner pool as a raw *sql.DB, used by the vendor→tenant
// resolver to map a session's active org to a tenant before any GUC is bound.
func NewTenantMiddleware(appDB *sql.DB, privileged *ent.Client, privilegedDB *sql.DB, bootstrapSlug string, platformHosts []string, headerTrusted bool, logger *zap.Logger) *TenantMiddleware {
	hosts := make(map[string]struct{}, len(platformHosts))
	for _, h := range platformHosts {
		hosts[strings.ToLower(h)] = struct{}{}
	}
	return &TenantMiddleware{
		appDB:         appDB,
		privileged:    privileged,
		privilegedDB:  privilegedDB,
		bootstrapSlug: bootstrapSlug,
		platformHosts: hosts,
		headerTrusted: headerTrusted,
		logger:        logger,
	}
}

// resolution is the outcome of host-based tenant resolution.
type resolution struct {
	tenantID   string
	hasTenant  bool
	isPlatform bool // platform host -> allowed no-tenant context
}

// Resolve resolves the tenant for the request from the public Host and stores
// the outcome in context. Precedence: (1) trusted X-Tenant-Id header
// (internal/test only); (2) tenant_domain hostname lookup; (3) platform host ->
// no-tenant platform context; (4) bootstrap slug dev fallback. An unknown host
// is rejected with 404 before any transaction opens. It does not open a
// transaction — Tx does.
func (m *TenantMiddleware) Resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		res, err := m.resolve(ctx, r)
		if err != nil {
			switch {
			case errors.Is(err, errInvalidTenantHeader):
				http.Error(w, "invalid tenant header", http.StatusBadRequest)
			case errors.Is(err, errUnknownHost):
				m.logger.Warn("tenant resolution: unknown host", zap.String("host", requestHost(r)))
				http.Error(w, "unknown host", http.StatusNotFound)
			default:
				m.logger.Warn("tenant resolution failed", zap.Error(err))
				http.Error(w, "tenant not resolved", http.StatusBadRequest)
			}
			return
		}

		if res.hasTenant {
			ctx = auth.WithTenantID(ctx, res.tenantID)
		} else if res.isPlatform {
			ctx = auth.WithPlatformContext(ctx)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *TenantMiddleware) resolve(ctx context.Context, r *http.Request) (resolution, error) {
	// (1) Internal/test escape hatch — only when explicitly trusted.
	if m.headerTrusted {
		if raw := r.Header.Get(TenantHeader); raw != "" {
			if !nanoid.Valid(raw) {
				return resolution{}, errInvalidTenantHeader
			}
			t, err := m.privileged.Tenant.Query().Where(tenant.IDEQ(raw)).Only(ctx)
			if err != nil {
				return resolution{}, err
			}
			return resolution{tenantID: t.ID, hasTenant: true}, nil
		}
	}

	host := requestHost(r)

	// (2) Host -> tenant via tenant_domain (privileged: no GUC set yet).
	// Only verified rows resolve: a BYO custom domain must prove ownership
	// (CNAME + TXT) before it routes, otherwise a tenant could claim a host it
	// does not control and have traffic attributed to it. Platform-assigned
	// hosts are verified by construction (see the backfill migration).
	if host != "" {
		d, err := m.privileged.TenantDomain.Query().
			Where(
				tenantdomain.HostnameEQ(host),
				tenantdomain.VerifiedAtNotNil(),
			).
			Only(ctx)
		if err == nil {
			return resolution{tenantID: d.TenantID, hasTenant: true}, nil
		}
		if !ent.IsNotFound(err) {
			return resolution{}, err
		}

		// (3) Platform host -> no-tenant platform context.
		if _, ok := m.platformHosts[host]; ok {
			return resolution{isPlatform: true}, nil
		}
	}

	// (4) Dev fallback to the bootstrap tenant slug.
	if m.bootstrapSlug != "" {
		t, err := m.privileged.Tenant.Query().Where(tenant.SlugEQ(m.bootstrapSlug)).Only(ctx)
		if err != nil {
			return resolution{}, err
		}
		return resolution{tenantID: t.ID, hasTenant: true}, nil
	}

	return resolution{}, errUnknownHost
}

// requestHost returns the lowercased, port-stripped public host for the
// request: X-Forwarded-Host (set by the web proxy) if present, else r.Host.
func requestHost(r *http.Request) string {
	h := r.Header.Get(ForwardedHostHeader)
	if h == "" {
		h = r.Host
	}
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	// A forwarded host may carry a comma-separated list; take the first.
	if i := strings.IndexByte(h, ','); i >= 0 {
		h = strings.TrimSpace(h[:i])
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(h)
}

var (
	errInvalidTenantHeader = errors.New("invalid X-Tenant-Id header")
	errUnknownHost         = errors.New("no tenant resolvable for host")
	// errVendorTenantDenied is surfaced as an undifferentiated 403 (unknown org,
	// unlinked org, and non-member all map to it) so the boundary can't be probed.
	errVendorTenantDenied = errors.New("vendor tenant resolution denied")
)

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{Code: code, Message: message})
}

// ResolveVendorTenant scopes a platform-host request to the logged-in vendor's
// tenant, for the account route group (after RequireAuth). The session's
// active_organization_id is only a hint; the privileged membership lookup is the
// authorization boundary and fails closed. is_platform_admin stays false for a
// vendor, so this only ever narrows access.
func (m *TenantMiddleware) ResolveVendorTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// On a tenant host app.tenant_id is already bound, so re-binding from the
		// session would be ambiguous: the account routes are platform-host only.
		if !enttenant.IsPlatform(ctx) {
			writeJSONError(w, http.StatusBadRequest, "not_platform_host",
				"Account routes are only available on the platform host")
			return
		}

		userID, ok := auth.GetUserID(ctx)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
			return
		}

		orgID, ok := auth.GetActiveOrgID(ctx)
		if !ok {
			writeJSONError(w, http.StatusForbidden, "no_active_organization",
				"No active organization for this session")
			return
		}

		tenantID, err := m.resolveVendorTenant(ctx, orgID, userID)
		if err != nil {
			if errors.Is(err, errVendorTenantDenied) {
				writeJSONError(w, http.StatusForbidden, "tenant_access_denied",
					"You are not a member of an organization linked to a tenant")
				return
			}
			m.logger.Error("vendor tenant resolution failed", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}

		tx, ok := repository.RequestTxFromContext(ctx)
		if !ok {
			m.logger.Error("vendor tenant resolution: no request tx in context")
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
			m.logger.Error("vendor tenant resolution: rebind tenant guc", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}

		ctx = auth.WithTenantID(ctx, tenantID)
		ctx = repository.ContextWithTenantID(ctx, tenantID)
		ctx = enttenant.WithTenant(ctx, tenantID)
		// A concrete tenant is now bound, so drop the no-tenant platform marker.
		ctx = enttenant.WithoutPlatform(ctx)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveVendorTenant returns the tenant the user may act on, or
// errVendorTenantDenied. It uses the privileged pool because no tenant is bound
// yet (an app-pool read would be denied by the org-keyed RLS); the JOIN enforces
// membership: a row comes back only for a member of the linked org.
func (m *TenantMiddleware) resolveVendorTenant(ctx context.Context, orgID, userID string) (string, error) {
	var tenantID string
	err := m.privilegedDB.QueryRowContext(ctx, `
		SELECT t.id
		FROM tenant t
		JOIN member m ON m.organization_id = t.organization_id
		WHERE t.organization_id = $1 AND m.user_id = $2
	`, orgID, userID).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errVendorTenantDenied
	}
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

// Tx opens a Postgres transaction on the app pool, binds it to the resolved
// tenant via set_config('app.tenant_id', ...), wires a tenant-guarded Ent
// client onto the request context, and commits on success / rolls back on
// error or panic. Resolve must run before this.
//
// Two modes:
//   - tenant request: app.tenant_id is the resolved tenant; the Ent guard
//     filters to it.
//   - platform request (platform host, no tenant): app.tenant_id is set to ”
//     (so app_current_tenant_id() returns NULL) and app.is_platform_admin is
//     seeded false. The auth middleware flips is_platform_admin to true only
//     after proving a platform admin. Until then RLS denies everything, so a
//     non-admin platform request is fail-closed at the database layer.
func (m *TenantMiddleware) Tx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		tenantID, hasTenant := auth.GetTenantID(ctx)
		isPlatform := auth.IsPlatformContext(ctx)
		if !hasTenant && !isPlatform {
			http.Error(w, "tenant not resolved", http.StatusBadRequest)
			return
		}

		conn, err := m.appDB.Conn(ctx)
		if err != nil {
			m.logger.Error("tenant tx: acquire conn", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = conn.Close() }()

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			m.logger.Error("tenant tx: begin", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// '' for the platform path -> app_current_tenant_id() returns NULL.
		tenantGUC := ""
		if hasTenant {
			tenantGUC = tenantID
		}
		if _, err := tx.ExecContext(ctx,
			"SELECT set_config('app.tenant_id', $1, true), set_config('app.is_platform_admin', 'false', true)",
			tenantGUC,
		); err != nil {
			_ = tx.Rollback()
			m.logger.Error("tenant tx: set_config", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
		enttenant.Register(txClient)

		ctx = repository.ContextWithClient(ctx, txClient)
		ctx = repository.ContextWithRequestTx(ctx, tx)
		if hasTenant {
			ctx = repository.ContextWithTenantID(ctx, tenantID)
			ctx = enttenant.WithTenant(ctx, tenantID)
		} else {
			ctx = enttenant.WithPlatform(ctx)
		}

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		committed := false
		defer func() {
			if p := recover(); p != nil {
				_ = tx.Rollback()
				panic(p)
			}
			if !committed {
				_ = tx.Rollback()
			}
		}()

		next.ServeHTTP(ww, r.WithContext(ctx))

		if status := ww.Status(); status == 0 || status < http.StatusBadRequest {
			if err := tx.Commit(); err != nil {
				m.logger.Error("tenant tx: commit", zap.Error(err))
				return
			}
			committed = true
		}
	})
}
