"use client"

import { authClient } from "@/lib/auth/client"
import { isMember, isStaff, isTenantAdmin, type MemberTier, tierAllows } from "@/lib/auth/rbac"

export function useMemberTier(): {
  tier: MemberTier | null
  isLoading: boolean
  isMember: boolean
  isStaff: boolean
  isTenantAdmin: boolean
  tierAllows: (required: MemberTier) => boolean
} {
  const { data, isPending } = authClient.useActiveMember()

  const raw = data?.role ?? null
  const tier: MemberTier | null =
    raw === "owner" || raw === "admin" || raw === "staff" || raw === "catalog" ? (raw as MemberTier) : null

  return {
    tier,
    isLoading: isPending,
    isMember: isMember(tier),
    isStaff: isStaff(tier),
    isTenantAdmin: isTenantAdmin(tier),
    tierAllows: (required: MemberTier) => tierAllows(tier, required),
  }
}
