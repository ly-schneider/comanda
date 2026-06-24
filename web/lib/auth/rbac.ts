export type MemberTier = "owner" | "admin" | "staff" | "catalog"

const VALID_TIERS = new Set<string>(["owner", "admin", "staff", "catalog"])

export const ROLE_LABELS_DE: Record<string, string> = {
  owner: "Inhaber",
  admin: "Administrator",
  staff: "Mitarbeiter",
  catalog: "Sortiment",
}

// owner is the singleton org creator and satisfies the admin tier.
export function isTenantAdmin(tier: MemberTier | null | undefined): boolean {
  return tier === "owner" || tier === "admin"
}

// staff tier: admin + staff (everything except catalog-only).
export function isStaff(tier: MemberTier | null | undefined): boolean {
  return tier === "owner" || tier === "admin" || tier === "staff"
}

// Any valid member tier (gates entry to /admin).
export function isMember(tier: MemberTier | null | undefined): boolean {
  return tier != null && VALID_TIERS.has(tier)
}

// Ordinal tier rank — higher = more privileged.
const TIER_RANK: Record<string, number> = {
  owner: 3,
  admin: 3,
  staff: 2,
  catalog: 1,
}

// Returns true when the given tier satisfies the required minimum tier.
export function tierAllows(tier: MemberTier | null | undefined, required: MemberTier): boolean {
  if (!tier) return false
  return (TIER_RANK[tier] ?? 0) >= (TIER_RANK[required] ?? 0)
}
