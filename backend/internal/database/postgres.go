package database

import (
	"crypto/x509"
	"database/sql"
	"fmt"
	"os"

	"backend/internal/config"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/trace"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

// PrivilegedClient is the table-owning, RLS-bypassing Ent client. It is used
// intentionally for cross-tenant work: migrations, cron, auth-bootstrap, and
// tenant resolution. It must never serve tenant request traffic.
type PrivilegedClient struct {
	Client *ent.Client
	DB     *sql.DB
}

// AppClient is the RLS-bound Ent client for request traffic. Without a tenant
// GUC set in a transaction it returns zero rows (RLS) and the Ent tenant guard
// denies scoped operations — it fails closed.
type AppClient struct {
	Client *ent.Client
	DB     *sql.DB
}

// openPool opens a database/sql pool for dsn. maxConns/minConns are passed
// explicitly so the app and privileged pools can be sized independently: the
// aggregate across every pod and pool must stay under Postgres max_connections,
// and keeping idle (minConns) close to maxConns avoids reconnect churn (each
// reconnect is a full TLS handshake to the managed instance).
func openPool(dsn string, cfg config.Config, maxConns, minConns int) (*sql.DB, error) {
	connConfig, err := buildConnConfig(dsn, cfg.Postgres.CAPath)
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*connConfig)
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(minConns)
	db.SetConnMaxLifetime(cfg.Postgres.MaxConnLifetime)
	db.SetConnMaxIdleTime(cfg.Postgres.MaxConnIdleTime)
	return db, nil
}

// buildConnConfig parses the DSN and, when caPath is set, pins that CA onto the
// connection's TLS config.
//
// Scaleway managed PG presents a per-instance, self-signed CA that isn't in the
// system roots. The DSN carries sslmode=verify-full but no sslrootcert (the CA
// is shipped as a mounted file instead), so without pinning it here verify-full
// fails with "certificate signed by unknown authority".
func buildConnConfig(dsn, caPath string) (*pgx.ConnConfig, error) {
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database DSN: %w", err)
	}
	if caPath == "" {
		return connConfig, nil
	}

	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read database CA %q: %w", caPath, err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("database CA %q: no certificates parsed", caPath)
	}
	if connConfig.TLSConfig != nil {
		connConfig.TLSConfig.RootCAs = roots
	}
	for _, fb := range connConfig.Fallbacks {
		if fb.TLSConfig != nil {
			fb.TLSConfig.RootCAs = roots
		}
	}
	return connConfig, nil
}

// NewPrivilegedClient opens the owner pool (DATABASE_URL).
func NewPrivilegedClient(cfg config.Config) (*PrivilegedClient, error) {
	db, err := openPool(cfg.Postgres.DSN, cfg, cfg.Postgres.PrivMaxConns, cfg.Postgres.PrivMinConns)
	if err != nil {
		zap.L().Error("failed to open privileged database", zap.Error(err))
		return nil, fmt.Errorf("failed to open privileged database: %w", err)
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))
	client.Intercept(trace.QueryInterceptor())
	client.Use(trace.MutationHook())

	zap.L().Info("privileged Ent client initialized")
	return &PrivilegedClient{Client: client, DB: db}, nil
}

// NewAppClient opens the RLS-bound app pool (DATABASE_URL_APP). The tenant
// guards are registered so any use without a tenant in context fails closed.
//
// RLS is the non-negotiable isolation boundary, so a missing DATABASE_URL_APP
// is fatal: silently falling back to the privileged DSN would route all request
// traffic through a BYPASSRLS connection and disable tenant isolation. The
// single-DSN fallback is allowed only with an explicit ALLOW_SINGLE_DSN=true
// opt-in, intended for local dev.
func NewAppClient(cfg config.Config) (*AppClient, error) {
	dsn := cfg.Postgres.AppDSN
	if dsn == "" {
		if !cfg.Postgres.AllowSingleDSN {
			return nil, fmt.Errorf(
				"DATABASE_URL_APP is required: the app pool must use the RLS-scoped role; " +
					"set DATABASE_URL_APP, or set ALLOW_SINGLE_DSN=true for local dev only",
			)
		}
		dsn = cfg.Postgres.DSN
		zap.L().Warn("ALLOW_SINGLE_DSN set; app client falling back to DATABASE_URL — RLS role separation is DISABLED (local dev only)")
	}

	db, err := openPool(dsn, cfg, cfg.Postgres.MaxConns, cfg.Postgres.MinConns)
	if err != nil {
		zap.L().Error("failed to open app database", zap.Error(err))
		return nil, fmt.Errorf("failed to open app database: %w", err)
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))
	client.Intercept(trace.QueryInterceptor())
	client.Use(trace.MutationHook())
	enttenant.Register(client)

	zap.L().Info("app Ent client initialized")
	return &AppClient{Client: client, DB: db}, nil
}
