// Package plugin is the in-tree extension framework: first-party plugins are
// compiled into the shared binary and stay dormant until a platform admin
// enables them for a tenant. A disabled plugin is invisible — its routes 404
// and it contributes nothing to the public plugin list.
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

// Plugin is a compiled-in feature module. Key is the stable identifier used in
// URLs, the tenant_plugin table, and analytics; Name is the platform-admin
// display label. Module contributes the plugin's fx providers; Routes returns a
// mount function wired under /v1/plugins/<key>.
type Plugin interface {
	Key() string
	Name() string
	Module() fx.Option
	Routes(wrapper *generated.ServerInterfaceWrapper, mw Middlewares) func(chi.Router)
}

// Middlewares hands a plugin the core request gates so it composes them per
// route instead of reaching into the router. Assembled in the HTTP router.
type Middlewares struct {
	RequireEnabled    func(http.Handler) http.Handler
	RequireDevice     func(auth.DeviceType) func(http.Handler) http.Handler
	RequireAuth       func() func(http.Handler) http.Handler
	OptionalAuth      func() func(http.Handler) http.Handler
	RequireTenantRole func(auth.TenantTier) func(http.Handler) http.Handler
	CSRF              func(http.Handler) http.Handler
}

// PaymentHook is the payment extension point. A plugin implementing it is
// dispatched during order payment: ValidatePayment before recording, and
// OnPaymentRecorded after. Wired by the payment service in a later ticket.
type PaymentHook interface {
	ValidatePayment(ctx context.Context, order *ent.Order, data json.RawMessage) error
	OnPaymentRecorded(ctx context.Context, order *ent.Order, method string, data json.RawMessage) error
}

// Starter is a plugin that runs a long-lived background loop, started on boot.
type Starter interface {
	Start(ctx context.Context) error
}
