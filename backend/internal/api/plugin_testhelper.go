package api

import (
	"backend/internal/plugin"

	"go.uber.org/zap"
)

// Test-only: drives the plugin handlers without the full fx graph.
func NewPluginHandlers(plugins plugin.Service, logger *zap.Logger) *Handlers {
	return &Handlers{
		plugins: plugins,
		logger:  logger,
	}
}
