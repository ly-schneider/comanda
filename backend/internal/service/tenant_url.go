package service

import (
	"context"
	"strings"

	"backend/internal/generated/ent"

	"go.uber.org/zap"
)

// TenantDomainLister is the narrow read surface needed to resolve a tenant's
// public host. Satisfied by repository.TenantDomainRepository.
type TenantDomainLister interface {
	List(ctx context.Context) ([]*ent.TenantDomain, error)
}

// TenantPrimaryHost returns the tenant's primary verified hostname, falling
// back to the first verified one. Empty when the tenant has no verified domain
// or the lookup fails (callers fall back to the platform base URL).
func TenantPrimaryHost(ctx context.Context, domains TenantDomainLister, logger *zap.Logger) string {
	if domains == nil {
		return ""
	}
	rows, err := domains.List(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to load tenant domains; using platform base", zap.Error(err))
		}
		return ""
	}
	host := ""
	for _, d := range rows {
		if d.VerifiedAt == nil {
			continue
		}
		if d.IsPrimary {
			return d.Hostname
		}
		if host == "" {
			host = d.Hostname
		}
	}
	return host
}

// TenantBaseURL builds the origin for tenant-facing links. Links must target
// the tenant's own verified host, not the platform apex (which has no tenant
// context); PublicBaseURL is only a fallback for tenants without a verified
// domain yet.
func TenantBaseURL(ctx context.Context, domains TenantDomainLister, publicBaseURL string, logger *zap.Logger) string {
	if host := TenantPrimaryHost(ctx, domains, logger); host != "" {
		return "https://" + host
	}
	base := strings.TrimRight(publicBaseURL, "/")
	if base == "" {
		base = "http://localhost:3000"
	}
	return base
}
