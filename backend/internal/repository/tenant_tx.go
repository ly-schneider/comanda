package repository

import (
	"context"
	stdsql "database/sql"
	"fmt"

	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
)

// RunInTenantTx runs fn inside a Postgres transaction on the app pool bound to
// tenantID, mirroring the request tenant-tx middleware: it sets the
// app.tenant_id GUC so RLS scopes every statement, wires a tenant-guarded Ent
// client onto the context (so repositories pick it up via ClientFromContext),
// and commits on success / rolls back on error or panic.
//
// Use it for tenant-scoped work off the request path (background goroutines,
// jobs): without the GUC set on the connection, RLS denies every row and the
// Ent tenant guard rejects the operation.
func RunInTenantTx(ctx context.Context, db *stdsql.DB, tenantID string, fn func(ctx context.Context) error) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("tenant tx: acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tenant tx: begin: %w", err)
	}

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

	if _, err = tx.ExecContext(ctx,
		"SELECT set_config('app.tenant_id', $1, true), set_config('app.is_platform_admin', 'false', true)",
		tenantID,
	); err != nil {
		return fmt.Errorf("tenant tx: set_config: %w", err)
	}

	txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
	enttenant.Register(txClient)

	fctx := ContextWithClient(ctx, txClient)
	fctx = ContextWithRequestTx(fctx, tx)
	fctx = ContextWithTenantID(fctx, tenantID)
	fctx = enttenant.WithTenant(fctx, tenantID)

	if err = fn(fctx); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("tenant tx: commit: %w", err)
	}
	committed = true
	return nil
}
