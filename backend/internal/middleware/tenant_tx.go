package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenant"
	"backend/internal/repository"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TenantHeader is the dev/stopgap header carrying the tenant UUID. Host-based
// resolution replaces this in P2; the tx plumbing it feeds stays.
const TenantHeader = "X-Tenant-Id"

// TenantMiddleware resolves the request's tenant and runs the request inside a
// single tenant-scoped Postgres transaction.
type TenantMiddleware struct {
	appDB         *sql.DB
	privileged    *ent.Client
	bootstrapSlug string
	logger        *zap.Logger
}

// NewTenantMiddleware builds the middleware. appDB is the RLS-bound app pool;
// privileged resolves the tenant table before any tenant is set.
func NewTenantMiddleware(appDB *sql.DB, privileged *ent.Client, bootstrapSlug string, logger *zap.Logger) *TenantMiddleware {
	return &TenantMiddleware{
		appDB:         appDB,
		privileged:    privileged,
		bootstrapSlug: bootstrapSlug,
		logger:        logger,
	}
}

// Resolve resolves the tenant for the request (X-Tenant-Id header, else the
// configured bootstrap tenant slug) and stores it in context. It does not open
// a transaction — TenantTx does. Requests it cannot resolve a tenant for are
// rejected, so no traffic reaches the tenant-scoped tx without a tenant.
func (m *TenantMiddleware) Resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		tenantID, err := m.resolveTenantID(ctx, r)
		if err != nil {
			m.logger.Warn("tenant resolution failed", zap.Error(err))
			http.Error(w, "tenant not resolved", http.StatusBadRequest)
			return
		}

		ctx = auth.WithTenantID(ctx, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *TenantMiddleware) resolveTenantID(ctx context.Context, r *http.Request) (uuid.UUID, error) {
	if raw := r.Header.Get(TenantHeader); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, errInvalidTenantHeader
		}
		t, err := m.privileged.Tenant.Query().Where(tenant.IDEQ(id)).Only(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		return t.ID, nil
	}

	if m.bootstrapSlug == "" {
		return uuid.Nil, errNoTenantResolvable
	}
	t, err := m.privileged.Tenant.Query().Where(tenant.SlugEQ(m.bootstrapSlug)).Only(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return t.ID, nil
}

var (
	errInvalidTenantHeader = errors.New("invalid X-Tenant-Id header")
	errNoTenantResolvable  = errors.New("no tenant resolvable for request")
)

// Tx opens a Postgres transaction on the app pool, binds it to the resolved
// tenant via set_config('app.tenant_id', ...), wires a tenant-guarded Ent
// client onto the request context, and commits on success / rolls back on
// error or panic. Resolve must run before this.
func (m *TenantMiddleware) Tx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		tenantID, ok := auth.GetTenantID(ctx)
		if !ok {
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

		if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
			_ = tx.Rollback()
			m.logger.Error("tenant tx: set_config", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
		enttenant.Register(txClient)

		ctx = repository.ContextWithClient(ctx, txClient)
		ctx = repository.ContextWithTenantID(ctx, tenantID)
		ctx = enttenant.WithTenant(ctx, tenantID)

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
