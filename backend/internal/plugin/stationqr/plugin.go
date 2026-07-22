package stationqr

import (
	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/plugin"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Plugin struct {
	rt      *runtime
	limiter *rateLimiter
}

type runtime struct {
	svc    Service
	logger *zap.Logger
}

func New() *Plugin {
	return &Plugin{
		rt:      &runtime{},
		limiter: newRateLimiter(enrollRateWindow, enrollRateMax),
	}
}

func (p *Plugin) Key() string  { return Key }
func (p *Plugin) Name() string { return "Station QR" }

func (p *Plugin) Module() fx.Option {
	return fx.Options(
		fx.Provide(
			func(s coreservice.DeviceService) DeviceApprover { return s },
			func(r repository.DeviceRepository) DeviceUpserter { return r },
			func(r *repository.ProductRepository) ProductLookup { return r },
			func(r repository.TenantDomainRepository) TenantDomains { return r },
			NewPresetRepository,
			NewService,
		),
		fx.Invoke(func(s Service, logger *zap.Logger) {
			p.rt.svc = s
			p.rt.logger = logger
		}),
	)
}

func (p *Plugin) Routes(wrapper *generated.ServerInterfaceWrapper, mw plugin.Middlewares) func(chi.Router) {
	w := *wrapper
	w.Handler = &handlers{rt: p.rt}
	return func(r chi.Router) {
		r.Group(func(gr chi.Router) {
			gr.Use(mw.RequireAuth())
			gr.Use(mw.RequireTenantRole(auth.TierOps))
			gr.Use(mw.CSRF)
			gr.Get("/presets", w.StationqrListPresets)
			gr.Post("/presets", w.StationqrCreatePreset)
			gr.Patch("/presets/{presetId}", w.StationqrUpdatePreset)
			gr.Delete("/presets/{presetId}", w.StationqrDeletePreset)
			gr.Post("/presets/{presetId}/rotate", w.StationqrRotatePreset)
		})
		r.Group(func(gr chi.Router) {
			gr.Use(p.limiter.middleware)
			gr.Post("/enroll", w.StationqrEnroll)
		})
	}
}
