package database

import (
	"context"
	"fmt"

	"backend/internal/config"

	"github.com/jackc/pgx/v5"
)

// NewListenConn opens a dedicated pgx connection for LISTEN/NOTIFY, with the
// same CA pinning as the pooled connections. The caller must Close it.
//
// LISTEN holds a session open for the connection's lifetime, which a
// transaction-mode pooler (PgBouncer) cannot support — so this prefers the
// direct ListenDSN, falling back to the app/privileged DSNs for local dev and
// pooler-less deployments.
func NewListenConn(ctx context.Context, cfg config.Config) (*pgx.Conn, error) {
	dsn := cfg.Postgres.ListenDSN
	if dsn == "" {
		dsn = cfg.Postgres.AppDSN
	}
	if dsn == "" {
		dsn = cfg.Postgres.DSN
	}
	if dsn == "" {
		return nil, fmt.Errorf("no database DSN configured for inventory listener")
	}
	connConfig, err := buildConnConfig(dsn, cfg.Postgres.CAPath)
	if err != nil {
		return nil, err
	}
	return pgx.ConnectConfig(ctx, connConfig)
}
