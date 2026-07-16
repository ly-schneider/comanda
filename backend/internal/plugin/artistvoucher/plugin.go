package artistvoucher

import (
	"backend/internal/auth"
	"backend/internal/blobstore"
	"backend/internal/generated/api/generated"
	"backend/internal/plugin"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Plugin struct {
	rt *runtime
}

type runtime struct {
	svc      Service
	branding coreservice.SettingsService
	blob     *blobstore.Client
	logger   *zap.Logger
}

func New() *Plugin {
	return &Plugin{rt: &runtime{}}
}

func (p *Plugin) Key() string  { return Key }
func (p *Plugin) Name() string { return "Artist Voucher" }

func (p *Plugin) Module() fx.Option {
	return fx.Options(
		fx.Provide(
			func(r repository.CategoryRepository) CategoryLookup { return r },
			func(s coreservice.QRKeyService) KeyProvider { return s },
			NewVoucherRepository,
			NewService,
		),
		fx.Invoke(func(s Service, branding coreservice.SettingsService, blob *blobstore.Client, logger *zap.Logger) {
			p.rt.svc = s
			p.rt.branding = branding
			p.rt.blob = blob
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
			gr.Get("/vouchers", w.ArtistVoucherList)
			gr.Get("/vouchers/{voucherId}/print.pdf", w.ArtistVoucherPrint)
			gr.Post("/vouchers", w.ArtistVoucherCreate)
			gr.Post("/vouchers/{voucherId}/revoke", w.ArtistVoucherRevoke)
			gr.Delete("/vouchers/{voucherId}", w.ArtistVoucherDelete)
		})
		r.Group(func(gr chi.Router) {
			gr.Use(mw.RequireDevice(auth.DeviceTypeStation))
			gr.Get("/revocations", w.ArtistVoucherRevocations)
		})
	}
}
