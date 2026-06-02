package integration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/repository"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// rlsEnv holds the two pools the RLS tests use: a privileged pool (owner,
// bypasses RLS) to seed data, and an app pool (comanda_app, RLS-bound) to prove
// isolation. Mirrors iglesio's dual-pool integration setup.
type rlsEnv struct {
	priv *sql.DB
	app  *sql.DB
}

// newRLSEnv applies the Atlas migrations (via NewTestDB on the privileged DSN)
// and opens the app pool. Skips when the app DSN is not configured.
func newRLSEnv(t *testing.T) *rlsEnv {
	t.Helper()

	appDSN := os.Getenv("POSTGRES_TEST_DSN_APP")
	if appDSN == "" {
		t.Skip("POSTGRES_TEST_DSN_APP not set; skipping RLS tests")
	}

	// Drives migrations + skips if POSTGRES_TEST_DSN is unset.
	tdb := NewTestDB(t)
	t.Cleanup(tdb.Close)

	app, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatalf("open app pool: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.PingContext(ctx); err != nil {
		t.Fatalf("ping app pool: %v", err)
	}

	return &rlsEnv{priv: tdb.DB, app: app}
}

// seedTenant inserts a tenant via the privileged pool and returns its id.
func (e *rlsEnv) seedTenant(t *testing.T, slug string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := e.priv.Exec(
		`INSERT INTO tenant (id, slug, name) VALUES ($1, $2, $3)`,
		id, slug, slug,
	)
	if err != nil {
		t.Fatalf("seed tenant %s: %v", slug, err)
	}
	return id
}

// privExec runs a statement on the privileged pool (RLS bypassed).
func (e *rlsEnv) privExec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := e.priv.Exec(query, args...); err != nil {
		t.Fatalf("priv exec: %v\n%s", err, query)
	}
}

// withTenant opens an app-pool transaction, sets app.tenant_id, runs fn, and
// rolls back (read-only assertions need no commit). A nil tenantID leaves the
// GUC unset to exercise the fail-closed path. Mirrors iglesio's withRlsContext.
func (e *rlsEnv) withTenant(t *testing.T, tenantID *uuid.UUID, fn func(tx *sql.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.app.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("app begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	val := ""
	if tenantID != nil {
		val = tenantID.String()
	}
	if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", val); err != nil {
		t.Fatalf("set tenant guc: %v", err)
	}

	fn(tx)
}

// appRequestTx mirrors the per-request tenant tx middleware against the
// RLS-bound app pool: opens a tx, sets app.tenant_id, binds a tenant-guarded
// Ent client, and returns a context carrying that client + tenant plus the
// client and a cleanup that commits and closes the connection. Service/repo
// calls made with this context reproduce the production request path, so they
// catch RLS regressions (e.g. a nested tx on the static pool with no GUC).
func (e *rlsEnv) appRequestTx(t *testing.T, tenantID uuid.UUID) (context.Context, *ent.Client, func()) {
	t.Helper()
	ctx := context.Background()
	conn, err := e.app.Conn(ctx)
	if err != nil {
		t.Fatalf("app conn: %v", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("app begin tx: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		t.Fatalf("set tenant guc: %v", err)
	}

	client := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
	enttenant.Register(client)

	ctx = repository.ContextWithClient(ctx, client)
	ctx = repository.ContextWithTenantID(ctx, tenantID)
	ctx = enttenant.WithTenant(ctx, tenantID)

	cleanup := func() {
		_ = tx.Commit()
		_ = conn.Close()
	}
	return ctx, client, cleanup
}
