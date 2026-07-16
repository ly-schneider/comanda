import { apiRequest, createApiError } from "@/lib/api"
import { getCSRFToken } from "@/lib/csrf"

export type VoucherStatus = "scheduled" | "active" | "expired" | "revoked"

export interface VoucherCategory {
  id: string
  name: string
}

export interface ArtistVoucher {
  id: string
  label: string
  validFrom: string
  validUntil: string
  revokedAt: string | null
  categories: VoucherCategory[]
  token: string
  status: VoucherStatus
  createdAt: string
}

export interface CreateVoucherInput {
  label: string
  validFrom: string
  validUntil: string
  categoryIds: string[]
}

const BASE = "/v1/plugins/artist-voucher"

function bearer(token: string) {
  return { Authorization: `Bearer ${token}` }
}

export async function listVouchers(token: string): Promise<ArtistVoucher[]> {
  const res = await apiRequest<{ vouchers: ArtistVoucher[] }>(`${BASE}/vouchers`, {
    headers: bearer(token),
  })
  return res.vouchers ?? []
}

export async function createVoucher(token: string, input: CreateVoucherInput): Promise<ArtistVoucher> {
  return apiRequest<ArtistVoucher>(`${BASE}/vouchers`, {
    method: "POST",
    headers: bearer(token),
    body: JSON.stringify(input),
  })
}

export async function revokeVoucher(token: string, id: string): Promise<ArtistVoucher> {
  return apiRequest<ArtistVoucher>(`${BASE}/vouchers/${encodeURIComponent(id)}/revoke`, {
    method: "POST",
    headers: bearer(token),
  })
}

// Raw fetch: apiRequest would choke parsing the 204's empty body.
export async function deleteVoucher(token: string, id: string): Promise<void> {
  const res = await fetch(`/api${BASE}/vouchers/${encodeURIComponent(id)}`, {
    method: "DELETE",
    headers: { ...bearer(token), "X-CSRF": getCSRFToken() || "" },
  })
  if (res.ok || res.status === 204) return
  const body = (await res.json().catch(() => ({}))) as { code?: string; message?: string }
  throw createApiError(res.status, body.message || body.code || `HTTP ${res.status}`, body.code)
}

// Opened via window.open, so the backend route authenticates the session cookie
// (forwarded by the /api proxy), not a Bearer header.
export function voucherPrintUrl(voucherId: string, count: number): string {
  return `/api${BASE}/vouchers/${encodeURIComponent(voucherId)}/print.pdf?count=${count}`
}
