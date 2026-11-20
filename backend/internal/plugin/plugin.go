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

// RefundHook lets a plugin give back what an order consumed when that order is
// refunded or cancelled. It runs for every plugin key the order was paid with,
// after the refund rows are written and before the refund is marked succeeded.
type RefundHook interface {
	OnOrderRefunded(ctx context.Context, order *ent.Order, in RefundedOrder) error
}

// RefundedOrder describes an order as the refund being committed leaves it.
type RefundedOrder struct {
	// Cancelled is set when the whole order was cancelled.
	Cancelled bool
	// RefundedUnits counts the units refunded so far per order line id,
	// including the refund being committed. Empty for an arbitrary refund,
	// which names no lines.
	RefundedUnits map[string]int
}

type Starter interface {
	Start(ctx context.Context) error
}
