import { apiRequest } from "@/lib/api"

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

// Opened via window.open, so the backend route authenticates the session cookie
// (forwarded by the /api proxy), not a Bearer header.
export function voucherPrintUrl(voucherId: string, count: number): string {
  return `/api${BASE}/vouchers/${encodeURIComponent(voucherId)}/print.pdf?count=${count}`
}
