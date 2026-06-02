package database

import (
	"database/sql"
	"fmt"

	"backend/internal/config"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/trace"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
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

func openPool(dsn string, cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.Postgres.MaxConns)
	db.SetMaxIdleConns(cfg.Postgres.MinConns)
	db.SetConnMaxLifetime(cfg.Postgres.MaxConnLifetime)
	db.SetConnMaxIdleTime(cfg.Postgres.MaxConnIdleTime)
	return db, nil
}

// NewPrivilegedClient opens the owner pool (DATABASE_URL).
func NewPrivilegedClient(cfg config.Config) (*PrivilegedClient, error) {
	db, err := openPool(cfg.Postgres.DSN, cfg)
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

// NewAppClient opens the RLS-bound app pool (DATABASE_URL_APP, falling back to
// DATABASE_URL only when unset so single-DSN dev keeps working). The tenant
// guards are registered so any use without a tenant in context fails closed.
func NewAppClient(cfg config.Config) (*AppClient, error) {
	dsn := cfg.Postgres.AppDSN
	if dsn == "" {
		dsn = cfg.Postgres.DSN
		zap.L().Warn("DATABASE_URL_APP not set; app client falling back to DATABASE_URL (no RLS role separation)")
	}

	db, err := openPool(dsn, cfg)
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
