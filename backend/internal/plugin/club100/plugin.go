package club100

import (
	"context"
	"encoding/json"
	"fmt"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/plugin"
	"backend/internal/repository"

	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Plugin struct {
	rt *runtime
}

// runtime is filled by Module's fx.Invoke at startup and read at request time,
// so the same registry instance can implement PaymentHook without its deps.
type runtime struct {
	svc    Service
	logger *zap.Logger
}

func New() *Plugin {
	return &Plugin{rt: &runtime{}}
}

func (p *Plugin) Key() string  { return Key }
func (p *Plugin) Name() string { return "100 Club" }

func (p *Plugin) Module() fx.Option {
	return fx.Options(
		fx.Provide(
			func(logger *zap.Logger) ElvantoSearcher { return NewElvantoClient(logger) },
			func(p *repository.ProductRepository) ProductLookup { return p },
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
			gr.Get("/people", w.Club100ListPeople)
			gr.Get("/remaining/{elvantoPersonId}", w.Club100GetRemaining)
		})
		r.Group(func(gr chi.Router) {
			gr.Use(mw.RequireAuth())
			gr.Use(mw.RequireTenantRole(auth.TierAppAdmin))
			gr.Use(mw.CSRF)
			gr.Get("/settings", w.Club100GetSettings)
			gr.Put("/settings", w.Club100UpdateSettings)
		})
	}
}

type paymentData struct {
	ElvantoPersonID   string `json:"elvantoPersonId"`
	ElvantoPersonName string `json:"elvantoPersonName"`
	FreeQuantity      int    `json:"freeQuantity"`
}

func parsePaymentData(data json.RawMessage) (paymentData, error) {
	var pd paymentData
	if err := json.Unmarshal(data, &pd); err != nil {
		return pd, fmt.Errorf("invalid club100 payment data")
	}
	if pd.ElvantoPersonID == "" {
		return pd, fmt.Errorf("missing club100 member")
	}
	if pd.FreeQuantity <= 0 {
		return pd, fmt.Errorf("free quantity must be positive")
	}
	return pd, nil
}

func (p *Plugin) ValidatePayment(ctx context.Context, order *ent.Order, data json.RawMessage) error {
	pd, err := parsePaymentData(data)
	if err != nil {
		return err
	}
	if err := p.rt.svc.ValidateOrderForRedemption(ctx, order.ID); err != nil {
		return err
	}
	remaining, _, err := p.rt.svc.GetRemainingRedemptions(ctx, pd.ElvantoPersonID)
	if err != nil {
		return err
	}
	if remaining < pd.FreeQuantity {
		return fmt.Errorf("insufficient remaining redemptions: have %d, need %d", remaining, pd.FreeQuantity)
	}
	return nil
}

func (p *Plugin) OnPaymentRecorded(ctx context.Context, order *ent.Order, _ string, data json.RawMessage) error {
	pd, err := parsePaymentData(data)
	if err != nil {
		return err
	}
	return p.rt.svc.RecordRedemption(ctx, pd.ElvantoPersonID, pd.ElvantoPersonName, order.ID, pd.FreeQuantity)
}
