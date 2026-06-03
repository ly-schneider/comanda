package app

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/generated/ent"
	"backend/internal/middleware"

	"go.uber.org/zap"
)

// NewPrivilegedClient provides the table-owning, RLS-bypassing client used for
// migrations, cron, auth-bootstrap, and tenant resolution.
func NewPrivilegedClient(cfg config.Config) (*database.PrivilegedClient, error) {
	return database.NewPrivilegedClient(cfg)
}

// NewAppClient provides the RLS-bound app client/pool for request traffic.
func NewAppClient(cfg config.Config) (*database.AppClient, error) {
	return database.NewAppClient(cfg)
}

// NewEntClient is the default request-path Ent client injected into the
// repositories. It is the RLS-bound app client, so a route that forgets the
// tenant-tx middleware fails closed instead of leaking across tenants.
func NewEntClient(app *database.AppClient) *ent.Client {
	return app.Client
}

// NewTenantMiddleware wires the per-request tenant resolver + transaction
// middleware: app pool for traffic, privileged client to resolve tenants.
func NewTenantMiddleware(
	app *database.AppClient,
	privileged *database.PrivilegedClient,
	cfg config.Config,
	logger *zap.Logger,
) *middleware.TenantMiddleware {
	return middleware.NewTenantMiddleware(
		app.DB,
		privileged.Client,
		cfg.App.BootstrapTenantSlug,
		cfg.App.PlatformHosts,
		cfg.App.TenantHeaderTrusted,
		logger,
	)
}
