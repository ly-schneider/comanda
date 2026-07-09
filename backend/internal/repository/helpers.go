package repository

import (
	"context"
	stdsql "database/sql"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/bundleslotoption"
	"backend/internal/generated/ent/deviceproduct"
	"backend/internal/generated/ent/predicate"

	"entgo.io/ent/dialect/sql"
)

// txClientKey is the context key for an ent.Client scoped to a transaction.
type txClientKey struct{}

// requestTxKey carries the raw *sql.Tx for the per-request tenant transaction
// so middleware that runs inside it (e.g. the auth middleware setting the
// app.is_platform_admin GUC) can execute on the same transaction.
type requestTxKey struct{}

// ContextWithRequestTx returns a child context carrying the request tx.
func ContextWithRequestTx(ctx context.Context, tx *stdsql.Tx) context.Context {
	return context.WithValue(ctx, requestTxKey{}, tx)
}

// RequestTxFromContext returns the request tx stored in ctx, if any.
func RequestTxFromContext(ctx context.Context) (*stdsql.Tx, bool) {
	tx, ok := ctx.Value(requestTxKey{}).(*stdsql.Tx)
	return tx, ok
}

// tenantIDKey is the context key for the request's resolved tenant ID. It is
// set by the tenant-tx middleware alongside the GUC so repositories that need
// the tenant value (e.g. the per-tenant settings singleton) can read it
// without importing the auth package (which would form an import cycle).
type tenantIDKey struct{}

// ContextWithTenantID returns a child context carrying the given tenant ID.
func ContextWithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey{}, tenantID)
}

// TenantIDFromContext returns the tenant ID stored in ctx, if any.
func TenantIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(tenantIDKey{}).(string)
	return id, ok
}

// ContextWithClient returns a child context carrying the given ent.Client.
// Use this to propagate a transactional client through repository calls.
func ContextWithClient(ctx context.Context, client *ent.Client) context.Context {
	return context.WithValue(ctx, txClientKey{}, client)
}

// ClientFromContext returns the transaction-scoped ent.Client from ctx,
// falling back to the provided default client when none is present.
func ClientFromContext(ctx context.Context, fallback *ent.Client) *ent.Client {
	if c, ok := ctx.Value(txClientKey{}).(*ent.Client); ok {
		return c
	}
	return fallback
}

// entDescOpt returns an sql.OrderTermOption that sorts in descending order.
func entDescOpt() sql.OrderTermOption {
	return sql.OrderDesc()
}

// entDeviceProductDeviceID returns a predicate matching device_product rows
// by device_id.
func entDeviceProductDeviceID(id string) predicate.DeviceProduct {
	return deviceproduct.DeviceIDEQ(id)
}

// entDeviceProductProductID returns a predicate matching device_product rows
// by product_id.
func entDeviceProductProductID(id string) predicate.DeviceProduct {
	return deviceproduct.ProductIDEQ(id)
}

// entBundleSlotOptionBundleSlotID returns a predicate matching bundle_slot_option
// rows by bundle_slot_id.
func entBundleSlotOptionBundleSlotID(id string) predicate.BundleSlotOption {
	return bundleslotoption.BundleSlotIDEQ(id)
}

// entBundleSlotOptionProductID returns a predicate matching bundle_slot_option
// rows by option_product_id.
func entBundleSlotOptionProductID(id string) predicate.BundleSlotOption {
	return bundleslotoption.OptionProductIDEQ(id)
}
