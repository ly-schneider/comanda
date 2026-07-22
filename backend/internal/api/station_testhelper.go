package api

import (
	"backend/internal/service"

	"go.uber.org/zap"
)

// NewStationTestHandlers wires just the deps the station self-service handlers
// reach (settings gate, stations, products, and devices for self-logout)
// without the full FX graph.
func NewStationTestHandlers(
	settings service.SettingsService,
	stations service.StationService,
	products service.ProductService,
	devices service.DeviceService,
	logger *zap.Logger,
) *Handlers {
	return &Handlers{
		settings: settings,
		stations: stations,
		products: products,
		devices:  devices,
		logger:   logger,
	}
}
