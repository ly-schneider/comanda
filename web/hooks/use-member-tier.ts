"use client"

import { authClient } from "@/lib/auth/client"
import { isOps, isTenantAdmin, type MemberTier } from "@/lib/auth/rbac"

export function useMemberTier(): {
  tier: MemberTier | null
  isLoading: boolean
  isMember: boolean
  isTenantAdmin: boolean
} {
  const { data, isPending } = authClient.useActiveMember()

  const raw = data?.role ?? null
  const tier = raw === "owner" || raw === "admin" || raw === "ops" ? (raw as MemberTier) : null

  return {
    tier,
    isLoading: isPending,
    isMember: isOps(tier),
    isTenantAdmin: isTenantAdmin(tier),
  }
}
