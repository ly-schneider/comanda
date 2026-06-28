export type MemberTier = "owner" | "admin" | "app_admin" | "staff" | "catalog"

const VALID_TIERS = new Set<string>(["owner", "admin", "app_admin", "staff", "catalog"])

export const ROLE_LABELS_DE: Record<string, string> = {
  owner: "Inhaber",
  admin: "Administrator",
  app_admin: "App-Admin",
  staff: "Mitarbeiter",
  catalog: "Sortiment",
}

// The branding/domains/payments tier. owner is the singleton org creator and
// satisfies it alongside admin; app_admin and below are hard-blocked.
export function isTenantAdmin(tier: MemberTier | null | undefined): boolean {
  return tier === "owner" || tier === "admin"
}

// App management tier: owner/admin/app_admin — users, invites, food settings.
export function isAppAdmin(tier: MemberTier | null | undefined): boolean {
  return tier === "owner" || tier === "admin" || tier === "app_admin"
}

// staff tier and above (everything except catalog-only).
export function isStaff(tier: MemberTier | null | undefined): boolean {
  return isAppAdmin(tier) || tier === "staff"
}

// Any valid member tier (gates entry to /admin).
export function isMember(tier: MemberTier | null | undefined): boolean {
  return tier != null && VALID_TIERS.has(tier)
}

// Ordinal tier rank — higher = more privileged.
const TIER_RANK: Record<string, number> = {
  owner: 5,
  admin: 4,
  app_admin: 3,
  staff: 2,
  catalog: 1,
}

export function tierAllows(tier: MemberTier | null | undefined, required: MemberTier): boolean {
  if (!tier) return false
  return (TIER_RANK[tier] ?? 0) >= (TIER_RANK[required] ?? 0)
}
