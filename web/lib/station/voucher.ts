/**
 * The offline verdict for an artist-voucher scan. Unlike the pickup path (which
 * skips expiry — its append-only ledger is the replay barrier), a voucher's
 * valid_until IS enforced offline: it doubles as the expiry backstop.
 */

import { verifyVoucherToken, type VoucherPayload } from "@/lib/station/qr-verify"

export const ARTIST_VOUCHER_PLUGIN_KEY = "artist-voucher"

export type VoucherInvalidReason =
  | "not-ready" // verification material not cached yet
  | "wrong-tenant"
  | "bad-signature"
  | "malformed"
  | "bad-payload"
  | "unsupported-version"

export type VoucherVerdict =
  | { kind: "valid"; voucher: VoucherPayload }
  | { kind: "scheduled"; voucher: VoucherPayload }
  | { kind: "expired"; voucher: VoucherPayload }
  | { kind: "revoked"; voucher: VoucherPayload }
  | { kind: "invalid"; reason: VoucherInvalidReason }

export interface VoucherEvalContext {
  publicKey: string | null
  stationTenantId: string | null
  now: number // epoch ms
  revoked?: (voucherId: string) => boolean
}

export function evaluateVoucher(token: string, ctx: VoucherEvalContext): VoucherVerdict {
  if (!ctx.publicKey || !ctx.stationTenantId) return { kind: "invalid", reason: "not-ready" }

  const verified = verifyVoucherToken(token, ctx.publicKey)
  if (!verified.ok) return { kind: "invalid", reason: verified.reason }

  const voucher = verified.payload
  // No server backstop offline, so the station is the sole enforcer of scoping.
  if (voucher.tenantId !== ctx.stationTenantId) return { kind: "invalid", reason: "wrong-tenant" }

  // Revocation outranks the window: a revoked voucher is dead even inside it.
  if (ctx.revoked?.(voucher.voucherId)) return { kind: "revoked", voucher }

  const nowSec = Math.floor(ctx.now / 1000)
  if (nowSec < voucher.validFrom) return { kind: "scheduled", voucher }
  if (nowSec >= voucher.validUntil) return { kind: "expired", voucher }
  return { kind: "valid", voucher }
}
