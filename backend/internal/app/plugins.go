package app

import (
	"backend/internal/plugin"
	"backend/internal/plugin/club100"
	"backend/internal/plugin/stationqr"

	"go.uber.org/fx"
)

// lives here, not in package plugin: each plugin imports plugin, so the list there would cycle
func registeredPlugins() []plugin.Plugin {
	return []plugin.Plugin{
		club100.New(),
		stationqr.New(),
	}
}

func NewPlugins() fx.Option {
	all := registeredPlugins()
	opts := []fx.Option{
		fx.Provide(func() []plugin.Plugin { return all }),
		fx.Provide(plugin.NewService),
	}
	for _, p := range all {
		opts = append(opts, p.Module())
	}
	return fx.Options(opts...)
}
