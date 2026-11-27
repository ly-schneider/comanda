package membership

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/plugin"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// A guest's own phone reaches the claim endpoints without a session, so they
// get a window per address. Generous enough for someone retrying a bad scan,
// tight enough that nobody walks through a provider's card codes with it.
const (
	claimRateWindow = time.Minute
	claimRateMax    = 20
)

type Plugin struct {
	rt      *runtime
	limiter *plugin.RateLimiter
}

// runtime is filled by Module's fx.Invoke at startup and read at request time,
// so the same registry instance can implement the hooks without its deps.
type runtime struct {
	svc    Service
	logger *zap.Logger
}

func New() *Plugin {
	return &Plugin{
		rt:      &runtime{},
		limiter: plugin.NewRateLimiter(claimRateWindow, claimRateMax, "Zu viele Versuche. Bitte kurz warten."),
	}
}

func (p *Plugin) Key() string  { return Key }
func (p *Plugin) Name() string { return "Mitgliederkarten" }

func (p *Plugin) Module() fx.Option {
	return fx.Options(
		fx.Provide(
			func(logger *zap.Logger) Provider { return NewProvider(logger) },
			func(s coreservice.ProductService) ProductLookup { return s },
			func(r repository.InventoryLedgerRepository) StockLookup { return r },
			func(s coreservice.PaymentService) OrderMinter { return s },
			func(r repository.OrderRepository) OrderSettler { return r },
			NewRedemptionRepository,
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
			gr.Use(mw.RequireEnabled)
			gr.Use(mw.RequireDevice(auth.DeviceTypePOS))
			gr.Post("/lookup", w.MembershipLookup)
		})
		// The guest's own phone: no session, the card code is the credential.
		r.Group(func(gr chi.Router) {
			gr.Use(p.limiter.Middleware)
			gr.Post("/claim", w.MembershipClaimView)
			gr.Post("/claim/redeem", w.MembershipClaimRedeem)
		})
		r.Group(func(gr chi.Router) {
			gr.Use(mw.RequireAuth())
			gr.Use(mw.RequireTenantRole(auth.TierAppAdmin))
			gr.Use(mw.CSRF)
			gr.Get("/settings", w.MembershipGetSettings)
			gr.Put("/settings", w.MembershipUpdateSettings)
		})
	}
}

// paymentData is what the till attaches to the payment: who was scanned and how
// many units they are getting. Which units those are is decided here from the
// order, never by the client.
type paymentData struct {
	MemberRef  string `json:"memberRef"`
	MemberName string `json:"memberName"`
	Quantity   int    `json:"quantity"`
}

func parsePaymentData(data json.RawMessage) (paymentData, error) {
	var pd paymentData
	if err := json.Unmarshal(data, &pd); err != nil {
		return pd, fmt.Errorf("invalid membership payment data")
	}
	if pd.MemberRef == "" {
		return pd, fmt.Errorf("missing member")
	}
	if pd.Quantity <= 0 {
		return pd, fmt.Errorf("free quantity must be positive")
	}
	return pd, nil
}

func (p *Plugin) ValidatePayment(ctx context.Context, order *ent.Order, data json.RawMessage) error {
	pd, err := parsePaymentData(data)
	if err != nil {
		return err
	}
	_, err = p.rt.svc.ValidateOrderClaim(ctx, order.ID, pd.Quantity)
	return err
}

func (p *Plugin) OnPaymentRecorded(ctx context.Context, order *ent.Order, _ string, data json.RawMessage) error {
	pd, err := parsePaymentData(data)
	if err != nil {
		return err
	}
	items, err := p.rt.svc.ValidateOrderClaim(ctx, order.ID, pd.Quantity)
	if err != nil {
		return err
	}
	return p.rt.svc.Book(ctx, order.ID, pd.MemberRef, pd.MemberName, pd.Quantity, items)
}

func (p *Plugin) OnOrderRefunded(ctx context.Context, order *ent.Order, in plugin.RefundedOrder) error {
	return p.rt.svc.Release(ctx, order, in)
}
