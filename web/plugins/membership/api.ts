import { apiRequest } from "@/lib/api"
import { readErrorMessage } from "@/lib/http"

export interface MembershipMember {
  name: string
  groupName?: string
}

export interface MembershipEntitlement {
  remaining: number
  max: number
  period?: string
}

export type MembershipStatus = "ok" | "exhausted" | "ineligible" | "unknown"

export interface MembershipLookupResult {
  status: MembershipStatus
  canRedeem: boolean
  message: string
  memberRef?: string
  member?: MembershipMember
  entitlement?: MembershipEntitlement
}

export interface MembershipClaimProduct {
  id: string
  name: string
  image?: string | null
}

export interface MembershipClaimView {
  status: MembershipStatus
  canRedeem: boolean
  message: string
  label: string
  member?: MembershipMember
  entitlement?: MembershipEntitlement
  products: MembershipClaimProduct[]
}

export interface MembershipClaimResult {
  orderId: string
  qrPayload: string
  message: string
  member?: MembershipMember
  entitlement?: MembershipEntitlement
}

export interface MembershipSettings {
  label: string
  freeProductIds: string[]
  /// False when the member system could not be reached: these are the last
  /// values it sent, and a change cannot be saved right now.
  synced: boolean
}

export async function lookupMembershipCard(token: string, code: string): Promise<MembershipLookupResult> {
  return apiRequest<MembershipLookupResult>("/v1/plugins/membership/lookup", {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ code }),
  })
}

export async function getMembershipSettings(token: string): Promise<MembershipSettings> {
  return apiRequest<MembershipSettings>("/v1/plugins/membership/settings", {
    headers: { Authorization: `Bearer ${token}` },
  })
}

/// The member system owns the selection; this hands it one made here and
/// answers with what is now true there.
export async function updateMembershipSettings(token: string, freeProductIds: string[]): Promise<MembershipSettings> {
  return apiRequest<MembershipSettings>("/v1/plugins/membership/settings", {
    method: "PUT",
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ freeProductIds }),
  })
}

// The guest's own phone has no session and no device token: it goes through the
// web proxy, where these two paths are exempt from CSRF because the card code
// is the credential.
async function postPublic<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return (await res.json()) as T
}

export async function fetchMembershipClaimView(code: string): Promise<MembershipClaimView> {
  return postPublic<MembershipClaimView>("/api/v1/plugins/membership/claim", { code })
}

export async function redeemMembershipClaim(
  code: string,
  items: { productId: string; quantity: number }[]
): Promise<MembershipClaimResult> {
  return postPublic<MembershipClaimResult>("/api/v1/plugins/membership/claim/redeem", { code, items })
}
