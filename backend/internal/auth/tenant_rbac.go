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
	// TierOps covers day-to-day operations: products, orders, stations, POS,
	// jetons, devices, staff-meals, events.
	TierOps TenantTier = iota
	// TierTenantAdmin covers tenant administration: settings, payments, branding,
	// domains, users, invites — plus everything ops can do.
	TierTenantAdmin
)

// memberRoleSatisfies reports whether a member.role meets the minimum tier.
// owner and admin are the tenant-admin tier (and therefore also satisfy ops);
// ops satisfies only the ops tier. An unrecognised role satisfies nothing
// (fail-closed).
func memberRoleSatisfies(role string, min TenantTier) bool {
	switch role {
	case "owner", "admin":
		return true
	case "ops":
		return min == TierOps
	default:
		return false
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
