package repository

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenantdomain"
)

// TenantDomainRepository manages the per-tenant custom-domain rows. The
// tenant-scoped methods run on the request client (RLS + Ent guard). The
// privileged methods take an explicit client so background work (DNS
// verification, the ingress reconciler, the trusted-origins feed) can read
// across tenants without an RLS tenant predicate fighting the query.
type TenantDomainRepository interface {
	Create(ctx context.Context, hostname, verificationToken string) (*ent.TenantDomain, error)
	List(ctx context.Context) ([]*ent.TenantDomain, error)
	GetByID(ctx context.Context, id string) (*ent.TenantDomain, error)
	Delete(ctx context.Context, id string) error
	SetPrimary(ctx context.Context, id string) error
	// MarkVerified records a successful ownership check: stamps verified_at and
	// moves the cert lifecycle to pending. Idempotent re-stamps are fine.
	MarkVerified(ctx context.Context, id string) (*ent.TenantDomain, error)

	// ListUnverified returns every not-yet-verified domain across all tenants.
	// The caller must pass the privileged (RLS-bypassing) client.
	ListUnverified(ctx context.Context, privileged *ent.Client) ([]*ent.TenantDomain, error)
	// ListVerified returns every verified domain across all tenants. The caller
	// must pass the privileged (RLS-bypassing) client.
	ListVerified(ctx context.Context, privileged *ent.Client) ([]*ent.TenantDomain, error)
	// ListAllVerifiedHostnames returns the verified hostnames across all tenants.
	ListAllVerifiedHostnames(ctx context.Context, privileged *ent.Client) ([]string, error)
}

type tenantDomainRepo struct {
	client *ent.Client
}

func NewTenantDomainRepository(client *ent.Client) TenantDomainRepository {
	return &tenantDomainRepo{client: client}
}

func (r *tenantDomainRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *tenantDomainRepo) Create(ctx context.Context, hostname, verificationToken string) (*ent.TenantDomain, error) {
	created, err := r.ec(ctx).TenantDomain.Create().
		SetHostname(hostname).
		SetVerificationToken(verificationToken).
		Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *tenantDomainRepo) List(ctx context.Context) ([]*ent.TenantDomain, error) {
	rows, err := r.ec(ctx).TenantDomain.Query().
		Order(tenantdomain.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *tenantDomainRepo) GetByID(ctx context.Context, id string) (*ent.TenantDomain, error) {
	e, err := r.ec(ctx).TenantDomain.Get(ctx, id)
	if err != nil {
		return nil, translateError(err)
	}
	return e, nil
}

func (r *tenantDomainRepo) Delete(ctx context.Context, id string) error {
	return translateError(r.ec(ctx).TenantDomain.DeleteOneID(id).Exec(ctx))
}

// SetPrimary makes the given domain the tenant's only primary. The partial
// unique index tenant_domain_one_primary would reject two primaries, so the
// other rows are cleared first inside the request transaction.
func (r *tenantDomainRepo) SetPrimary(ctx context.Context, id string) error {
	client := r.ec(ctx)
	target, err := client.TenantDomain.Get(ctx, id)
	if err != nil {
		return translateError(err)
	}
	if _, err := client.TenantDomain.Update().
		Where(
			tenantdomain.TenantIDEQ(target.TenantID),
			tenantdomain.IsPrimary(true),
			tenantdomain.IDNEQ(id),
		).
		SetIsPrimary(false).
		Save(ctx); err != nil {
		return translateError(err)
	}
	return translateError(client.TenantDomain.UpdateOneID(id).SetIsPrimary(true).Exec(ctx))
}

func (r *tenantDomainRepo) MarkVerified(ctx context.Context, id string) (*ent.TenantDomain, error) {
	updated, err := r.ec(ctx).TenantDomain.UpdateOneID(id).
		SetVerifiedAt(time.Now()).
		SetCertStatus(tenantdomain.CertStatusPending).
		Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return updated, nil
}

func (r *tenantDomainRepo) ListUnverified(ctx context.Context, privileged *ent.Client) ([]*ent.TenantDomain, error) {
	rows, err := privileged.TenantDomain.Query().
		Where(tenantdomain.VerifiedAtIsNil()).
		Order(tenantdomain.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *tenantDomainRepo) ListVerified(ctx context.Context, privileged *ent.Client) ([]*ent.TenantDomain, error) {
	rows, err := privileged.TenantDomain.Query().
		Where(tenantdomain.VerifiedAtNotNil()).
		Order(tenantdomain.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *tenantDomainRepo) ListAllVerifiedHostnames(ctx context.Context, privileged *ent.Client) ([]string, error) {
	rows, err := r.ListVerified(ctx, privileged)
	if err != nil {
		return nil, err
	}
	hostnames := make([]string, 0, len(rows))
	for _, row := range rows {
		hostnames = append(hostnames, row.Hostname)
	}
	return hostnames, nil
}
