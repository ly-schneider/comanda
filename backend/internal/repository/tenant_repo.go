package repository

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenant"
)

// TenantRepository manages the platform's tenant roster plus the per-tenant rows
// a new tenant needs to function (its settings singleton and primary domain).
// Every method runs on the request Ent client; in a platform-admin context that
// client is RLS-bypassed via the app.is_platform_admin GUC, so these operate
// cross-tenant. The scoped seed rows (settings, tenant_domain) carry an explicit
// tenant_id because the platform context never auto-stamps scoped writes.
type TenantRepository interface {
	Create(ctx context.Context, slug, name string) (*ent.Tenant, error)
	List(ctx context.Context) ([]*ent.Tenant, error)
	GetByID(ctx context.Context, id string) (*ent.Tenant, error)
	SetStatus(ctx context.Context, id string, status tenant.Status) (*ent.Tenant, error)
	// SeedSettings creates the per-tenant settings singleton (PK = tenant id) with
	// the schema's neutral branding defaults.
	SeedSettings(ctx context.Context, tenantID string) error
	// SeedPrimaryDomain creates a verified, platform-assigned primary domain.
	SeedPrimaryDomain(ctx context.Context, tenantID, hostname string) error
}

type tenantRepo struct {
	client *ent.Client
}

func NewTenantRepository(client *ent.Client) TenantRepository {
	return &tenantRepo{client: client}
}

func (r *tenantRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *tenantRepo) Create(ctx context.Context, slug, name string) (*ent.Tenant, error) {
	created, err := r.ec(ctx).Tenant.Create().
		SetSlug(slug).
		SetName(name).
		Save(ctx)
	return created, translateError(err)
}

func (r *tenantRepo) List(ctx context.Context) ([]*ent.Tenant, error) {
	list, err := r.ec(ctx).Tenant.Query().
		Order(tenant.ByCreatedAt()).
		All(ctx)
	return list, translateError(err)
}

func (r *tenantRepo) GetByID(ctx context.Context, id string) (*ent.Tenant, error) {
	t, err := r.ec(ctx).Tenant.Get(ctx, id)
	return t, translateError(err)
}

func (r *tenantRepo) SetStatus(ctx context.Context, id string, status tenant.Status) (*ent.Tenant, error) {
	updated, err := r.ec(ctx).Tenant.UpdateOneID(id).
		SetStatus(status).
		Save(ctx)
	return updated, translateError(err)
}

func (r *tenantRepo) SeedSettings(ctx context.Context, tenantID string) error {
	err := r.ec(ctx).Settings.Create().
		SetID(tenantID).
		SetTenantID(tenantID).
		Exec(ctx)
	return translateError(err)
}

func (r *tenantRepo) SeedPrimaryDomain(ctx context.Context, tenantID, hostname string) error {
	// Platform-assigned primary domain: verified by construction (served by the
	// wildcard cert) and with no verification token, which marks it platform-owned
	// so a tenant admin can't delete it. Mirrors the bootstrap-tenant seed.
	err := r.ec(ctx).TenantDomain.Create().
		SetTenantID(tenantID).
		SetHostname(hostname).
		SetIsPrimary(true).
		SetVerifiedAt(time.Now()).
		Exec(ctx)
	return translateError(err)
}
