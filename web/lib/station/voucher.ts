/**
 * The offline verdict for an artist-voucher scan: verify the 0x10 token, enforce
 * tenant scoping and the signed validity window against the device clock, then
 * classify the result. Purely local — a never-synced station renders a full
 * verdict from the token alone, which is what lets an admin issue a voucher on
 * the spot and have offline stations honour it immediately.
 *
 * Unlike the pickup path (which deliberately skips expiry — the append-only
 * ledger is its replay barrier), a voucher's `valid_until` IS enforced offline:
 * it doubles as the expiry backstop when the station can't reach the server.
 *
 * Revocation (the manual block list + server feed) is layered in by T4 through
 * the `revoked` predicate; absent it, nothing is revoked.
 */

import { verifyVoucherToken, type VoucherPayload } from "@/lib/station/qr-verify"

export const ARTIST_VOUCHER_PLUGIN_KEY = "artist-voucher"

export type VoucherInvalidReason =
  | "not-ready" // verification material (public key / tenant) not cached yet
  | "wrong-tenant" // valid signature, but issued by a different tenant
  | "bad-signature"
  | "malformed"
  | "bad-payload"
  | "unsupported-version"

export type VoucherVerdict =
  | { kind: "valid"; voucher: VoucherPayload }
  | { kind: "scheduled"; voucher: VoucherPayload } // before valid_from
  | { kind: "expired"; voucher: VoucherPayload } // at/after valid_until
  | { kind: "revoked"; voucher: VoucherPayload }
  | { kind: "invalid"; reason: VoucherInvalidReason }

export interface VoucherEvalContext {
  publicKey: string | null
  stationTenantId: string | null
  now: number // epoch ms (device clock)
  // T4 supplies the block-list / server-feed check; absent = nothing revoked.
  revoked?: (voucherId: string) => boolean
}

export function evaluateVoucher(token: string, ctx: VoucherEvalContext): VoucherVerdict {
  if (!ctx.publicKey || !ctx.stationTenantId) return { kind: "invalid", reason: "not-ready" }

  const verified = verifyVoucherToken(token, ctx.publicKey)
  if (!verified.ok) return { kind: "invalid", reason: verified.reason }

  const voucher = verified.payload
  // On the fully-offline path there is no server backstop, so the station is the
  // sole enforcer of tenant scoping.
  if (voucher.tenantId !== ctx.stationTenantId) return { kind: "invalid", reason: "wrong-tenant" }

  // Revocation outranks the window: a revoked voucher is dead even inside it.
  if (ctx.revoked?.(voucher.voucherId)) return { kind: "revoked", voucher }

  const nowSec = Math.floor(ctx.now / 1000)
  if (nowSec < voucher.validFrom) return { kind: "scheduled", voucher }
  if (nowSec >= voucher.validUntil) return { kind: "expired", voucher }
  return { kind: "valid", voucher }
}
