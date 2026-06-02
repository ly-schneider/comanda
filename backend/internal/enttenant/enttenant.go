// Package enttenant provides the Ent-layer, defense-in-depth tenant guard.
//
// Postgres row-level security is the primary isolation boundary (see the
// tenant-and-RLS migration). This package is a second wall at the ORM layer:
//
//   - Interceptor: injects a `tenant_id = <ctx tenant>` predicate on every
//     read of a tenant-scoped entity, and denies tenant-scoped reads that run
//     with no tenant in context.
//   - Hook: stamps `tenant_id` from context on create, rejects writes whose
//     explicit tenant_id disagrees with the context tenant, and denies any
//     tenant-scoped write with no tenant in context.
//
// The privileged client is registered WITHOUT these guards, so migrations,
// cron, and auth-bootstrap can operate across tenants intentionally.
package enttenant

import (
	"context"
	"errors"
	"fmt"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/intercept"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

// ErrNoTenant is returned when a tenant-scoped operation runs on the app
// client without a tenant in context. Fail-closed: better to error than leak.
var ErrNoTenant = errors.New("enttenant: tenant-scoped operation without a tenant in context")

// ErrTenantMismatch is returned when a write explicitly sets a tenant_id that
// differs from the tenant resolved for the request.
var ErrTenantMismatch = errors.New("enttenant: mutation tenant_id does not match request tenant")

const tenantColumn = "tenant_id"

// scopedTypes is the set of Ent type names that carry tenant_id (i.e. embed
// TenantMixin). Kept in lockstep with the schemas using the mixin. A type
// absent here is treated as global (no tenant filtering) — so adding a new
// tenant-scoped schema without listing it here is a leak; the RLS coverage
// test guards the DB side, and TestScopedTypesMatchSchema guards this side.
var scopedTypes = map[string]struct{}{
	ent.TypeProduct:                  {},
	ent.TypeCategory:                 {},
	ent.TypeJeton:                    {},
	ent.TypeMenuSlot:                 {},
	ent.TypeMenuSlotOption:           {},
	ent.TypeOrder:                    {},
	ent.TypeOrderLine:                {},
	ent.TypeOrderPayment:             {},
	ent.TypeOrderLineRedemption:      {},
	ent.TypeInventoryLedger:          {},
	ent.TypeClub100Redemption:        {},
	ent.TypeClub100FreeProduct:       {},
	ent.TypeSettings:                 {},
	ent.TypeIdempotency:              {},
	ent.TypeDevice:                   {},
	ent.TypeDeviceBinding:            {},
	ent.TypeDeviceProduct:            {},
	ent.TypeAdminInvite:              {},
	ent.TypeVolunteerCampaign:        {},
	ent.TypeVolunteerCampaignProduct: {},
	ent.TypeVolunteerRedemption:      {},
}

// IsScoped reports whether the named Ent type carries tenant_id.
func IsScoped(typeName string) bool {
	_, ok := scopedTypes[typeName]
	return ok
}

// ScopedTypeNames returns the tenant-scoped Ent type names.
func ScopedTypeNames() []string {
	names := make([]string, 0, len(scopedTypes))
	for n := range scopedTypes {
		names = append(names, n)
	}
	return names
}

type tenantCtxKey struct{}

// WithTenant binds a tenant ID to ctx for the Ent guards to read. The tenant-tx
// middleware calls this when building the per-request client.
func WithTenant(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

// TenantFromContext returns the tenant bound to ctx, if any.
func TenantFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(tenantCtxKey{}).(uuid.UUID)
	return id, ok
}

// Interceptor injects the tenant predicate on reads of scoped entities and
// fails closed when no tenant is present.
func Interceptor() ent.Interceptor {
	return intercept.TraverseFunc(func(ctx context.Context, q intercept.Query) error {
		if !IsScoped(q.Type()) {
			return nil
		}
		tenantID, ok := TenantFromContext(ctx)
		if !ok {
			return ErrNoTenant
		}
		q.WhereP(func(s *sql.Selector) {
			s.Where(sql.EQ(s.C(tenantColumn), tenantID))
		})
		return nil
	})
}

type tenantMutation interface {
	SetTenantID(uuid.UUID)
	TenantID() (uuid.UUID, bool)
}

// Hook stamps and validates tenant_id on writes of scoped entities.
func Hook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if !IsScoped(m.Type()) {
				return next.Mutate(ctx, m)
			}
			tenantID, ok := TenantFromContext(ctx)
			if !ok {
				return nil, ErrNoTenant
			}
			tm, ok := m.(tenantMutation)
			if !ok {
				return nil, fmt.Errorf("enttenant: %s is scoped but its mutation lacks SetTenantID", m.Type())
			}
			if existing, set := tm.TenantID(); set {
				if existing != tenantID {
					return nil, ErrTenantMismatch
				}
			} else {
				tm.SetTenantID(tenantID)
			}
			return next.Mutate(ctx, m)
		})
	}
}

// Register attaches the tenant guards to a client. Use this on the app
// (RLS-bound) request client only — never the privileged client.
func Register(client *ent.Client) {
	client.Intercept(Interceptor())
	client.Use(Hook())
}
