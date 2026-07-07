package api

import (
	"backend/internal/plugin"

	"go.uber.org/zap"
)

// NewPluginHandlers lets the plugin HTTP integration tests drive the generated
// ServerInterface plugin methods without standing up the full FX graph. The
// plugin handlers only reach h.plugins.
func NewPluginHandlers(plugins plugin.Service, logger *zap.Logger) *Handlers {
	return &Handlers{
		plugins: plugins,
		logger:  logger,
	}
}
