package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"backend/internal/config"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenant"
	"backend/internal/repository"
)

var (
	// ErrInvalidSlug signals a 400: the slug is not a valid DNS label.
	ErrInvalidSlug = errors.New("invalid_slug")
	// ErrReservedSlug signals a 400: the slug collides with a platform/reserved host.
	ErrReservedSlug = errors.New("reserved_slug")
	// ErrInvalidName signals a 400: the tenant display name is empty.
	ErrInvalidName = errors.New("invalid_name")
	// ErrInvalidStatus signals a 400: the status is not active|suspended.
	ErrInvalidStatus = errors.New("invalid_status")
)

// slugPattern matches a single lowercase DNS label: 1–63 chars of [a-z0-9-]
// without a leading/trailing hyphen. The slug becomes the tenant's subdomain.
var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// reservedSlugs can never be a tenant slug: they collide with platform hosts or
// reserved subdomains. Mirrors web/lib/tenant.ts RESERVED_SLUGS so a created
// tenant can never shadow the platform host.
var reservedSlugs = map[string]struct{}{
	"www": {}, "api": {}, "app": {}, "admin": {}, "platform": {},
	"staging": {}, "static": {}, "assets": {}, "cdn": {}, "mail": {},
	"smtp": {}, "ftp": {}, "ns": {}, "ns1": {}, "ns2": {},
}

type TenantService interface {
	Create(ctx context.Context, slug, name string) (*ent.Tenant, error)
	List(ctx context.Context) ([]*ent.Tenant, error)
	Get(ctx context.Context, id string) (*ent.Tenant, error)
	SetStatus(ctx context.Context, id, status string) (*ent.Tenant, error)
}

type tenantService struct {
	tenants repository.TenantRepository
	cfg     config.Config
}

func NewTenantService(tenants repository.TenantRepository, cfg config.Config) TenantService {
	return &tenantService{tenants: tenants, cfg: cfg}
}

func (s *tenantService) Create(ctx context.Context, slug, name string) (*ent.Tenant, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidName
	}
	if !slugPattern.MatchString(slug) {
		return nil, ErrInvalidSlug
	}
	if _, reserved := reservedSlugs[slug]; reserved {
		return nil, ErrReservedSlug
	}

	created, err := s.tenants.Create(ctx, slug, name)
	if err != nil {
		return nil, err
	}
	// The tenant, its settings singleton, and its primary domain are all created
	// inside the one per-request tx, so a later failure rolls the whole set back.
	if err := s.tenants.SeedSettings(ctx, created.ID); err != nil {
		return nil, err
	}
	if base := s.cfg.CustomDomain.BaseDomain; base != "" {
		if err := s.tenants.SeedPrimaryDomain(ctx, created.ID, slug+"."+base); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func (s *tenantService) List(ctx context.Context) ([]*ent.Tenant, error) {
	return s.tenants.List(ctx)
}

func (s *tenantService) Get(ctx context.Context, id string) (*ent.Tenant, error) {
	return s.tenants.GetByID(ctx, id)
}

func (s *tenantService) SetStatus(ctx context.Context, id, status string) (*ent.Tenant, error) {
	var st tenant.Status
	switch status {
	case string(tenant.StatusActive):
		st = tenant.StatusActive
	case string(tenant.StatusSuspended):
		st = tenant.StatusSuspended
	default:
		return nil, ErrInvalidStatus
	}
	return s.tenants.SetStatus(ctx, id, st)
}
