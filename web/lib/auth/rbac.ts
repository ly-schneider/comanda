export type MemberTier = "owner" | "admin" | "ops"

const VALID_TIERS = new Set<string>(["owner", "admin", "ops"])

export function isTenantAdmin(tier: MemberTier | null | undefined): boolean {
  return tier === "owner" || tier === "admin"
}

export function isOps(tier: MemberTier | null | undefined): boolean {
  return tier != null && VALID_TIERS.has(tier)
}
