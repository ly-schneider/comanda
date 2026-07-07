package app

import (
	"net/http"

	"backend/internal/api"
	"backend/internal/auth"
	"backend/internal/config"
	httpRouter "backend/internal/http"
	"backend/internal/middleware"
	"backend/internal/plugin"

	"go.uber.org/zap"
)

func NewRouter(
	apiHandlers *api.Handlers,
	betterAuthMw *auth.SessionMiddleware,
	deviceAuthMw *auth.DeviceAuthMiddleware,
	securityMw *middleware.SecurityMiddleware,
	systemMw *middleware.SystemDisableMiddleware,
	tenantMw *middleware.TenantMiddleware,
	plugins []plugin.Plugin,
	pluginSvc plugin.Service,
	logger *zap.Logger,
	cfg config.Config,
) http.Handler {
	isDev := cfg.App.AppEnv == "dev"

	return httpRouter.NewRouter(
		cfg,
		apiHandlers,
		betterAuthMw, deviceAuthMw, securityMw, systemMw, tenantMw,
		plugins, pluginSvc,
		logger, isDev,
	)
}
