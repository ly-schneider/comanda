package app

import (
	"backend/internal/plugin"

	"go.uber.org/fx"
)

func NewPlugins() fx.Option {
	opts := []fx.Option{
		fx.Provide(func() []plugin.Plugin { return plugin.All }),
		fx.Provide(plugin.NewService),
	}
	for _, p := range plugin.All {
		opts = append(opts, p.Module())
	}
	return fx.Options(opts...)
}
