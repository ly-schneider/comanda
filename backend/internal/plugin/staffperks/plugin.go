package staffperks

import (
	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/plugin"
	coreservice "backend/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Plugin struct {
	rt *runtime
}

type runtime struct {
	svc    Service
	logger *zap.Logger
}

func New() *Plugin {
	return &Plugin{rt: &runtime{}}
}

func (p *Plugin) Key() string  { return Key }
func (p *Plugin) Name() string { return "Staff Perks" }

func (p *Plugin) Module() fx.Option {
	return fx.Options(
		fx.Provide(
			NewVolunteerCampaignRepository,
			NewVolunteerRedemptionRepository,
			NewService,
			func(s Service) coreservice.CampaignClaimService { return s },
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
			gr.Get("/campaigns", w.ListVolunteerCampaigns)
			gr.Post("/campaigns", w.CreateVolunteerCampaign)
			gr.Get("/campaigns/{campaignId}", w.GetVolunteerCampaign)
			gr.Patch("/campaigns/{campaignId}", w.UpdateVolunteerCampaign)
			gr.Post("/campaigns/{campaignId}/end", w.EndVolunteerCampaign)
			gr.Post("/campaigns/{campaignId}/rotate-token", w.RotateVolunteerCampaignToken)
			gr.Get("/campaigns/{campaignId}/print.pdf", w.PrintStaffPerkSlips)
		})
	}
}
