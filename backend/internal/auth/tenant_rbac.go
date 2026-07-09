package auth

import (
	"context"
	"database/sql"
	"errors"

	"backend/internal/repository"
)

// TenantTier is the minimum membership tier a route requires. Authorization for
// the subdomain /admin surface is driven by the tenant-scoped org member.role,
// not the global user.role.
type TenantTier int

const (
	// TierCatalog is the lowest tier: products (incl. images and inventory),
	// categories, and menus only.
	TierCatalog TenantTier = iota
	// TierOps (the staff tier) covers day-to-day operations: orders, stations,
	// POS, jetons, devices, staff-perks, events — plus everything catalog can do.
	TierOps
	// TierAppAdmin (the app-admin tier) covers full app management — users,
	// invites, shop settings — but not branding, domains, or payments.
	TierAppAdmin
	// TierTenantAdmin covers branding, domains, payments, and the account hub.
	TierTenantAdmin
)

// memberRoleSatisfies reports whether a member.role meets the minimum tier. The
// tiers nest, so the role's rank is compared ordinally: owner and admin rank at
// tenant-admin, app_admin at app-admin, staff at ops, catalog at the lowest tier.
// An unrecognised role ranks below everything and satisfies nothing (fail-closed).
func memberRoleSatisfies(role string, min TenantTier) bool {
	rank, ok := roleRank(role)
	if !ok {
		return false
	}
	return rank >= min
}

func roleRank(role string) (TenantTier, bool) {
	switch role {
	case "owner", "admin":
		return TierTenantAdmin, true
	case "app_admin":
		return TierAppAdmin, true
	case "staff":
		return TierOps, true
	case "catalog":
		return TierCatalog, true
	default:
		return 0, false
	}
}

// tenantMemberRole reads the caller's membership role in the currently bound
// tenant's organization, on the request tx. The org-keyed RLS pins the lookup to
// the host-resolved tenant's org, so a non-member (or a member of a different
// tenant) yields ok=false. ok=false (never an error) is also returned when the
// user is absent or has no membership row; err is reserved for infrastructure
// failures (missing request tx, query error) so callers fail closed.
func tenantMemberRole(ctx context.Context) (role string, ok bool, err error) {
	userID, hasUser := GetUserID(ctx)
	if !hasUser {
		return "", false, nil
	}

	tx, hasTx := repository.RequestTxFromContext(ctx)
	if !hasTx {
		return "", false, errors.New("tenant rbac: no request tx in context")
	}

	scanErr := tx.QueryRowContext(ctx,
		"SELECT role FROM member WHERE organization_id = app_current_org_id() AND user_id = $1",
		userID,
	).Scan(&role)
	switch {
	case errors.Is(scanErr, sql.ErrNoRows):
		return "", false, nil
	case scanErr != nil:
		return "", false, scanErr
	}
	return role, true, nil
}

// HasTenantTier reports whether the caller may act at the given tier in the
// currently bound tenant. A platform admin always satisfies (consistent with the
// RLS bypass); otherwise it is driven purely by the tenant-scoped membership
// role, so the global user.role grants nothing cross-tenant.
func HasTenantTier(ctx context.Context, min TenantTier) (bool, error) {
	if IsPlatformAdmin(ctx) {
		return true, nil
	}
	role, ok, err := tenantMemberRole(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return memberRoleSatisfies(role, min), nil
}
