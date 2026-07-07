package plugin

import (
	"context"
	"encoding/json"
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"

	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
)

type Plugin interface {
	Key() string
	Name() string
	Module() fx.Option
	Routes(wrapper *generated.ServerInterfaceWrapper, mw Middlewares) func(chi.Router)
}

type Middlewares struct {
	RequireEnabled    func(http.Handler) http.Handler
	RequireDevice     func(auth.DeviceType) func(http.Handler) http.Handler
	RequireAuth       func() func(http.Handler) http.Handler
	OptionalAuth      func() func(http.Handler) http.Handler
	RequireTenantRole func(auth.TenantTier) func(http.Handler) http.Handler
	CSRF              func(http.Handler) http.Handler
}

type PaymentHook interface {
	ValidatePayment(ctx context.Context, order *ent.Order, data json.RawMessage) error
	OnPaymentRecorded(ctx context.Context, order *ent.Order, method string, data json.RawMessage) error
}

type Starter interface {
	Start(ctx context.Context) error
}
