package integration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

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
