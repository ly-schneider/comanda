import type { EntitlementPayload, VerifyFailure } from "@/lib/station/qr-verify"
import type { CatalogProduct } from "@/lib/station/redeem"
import type { VoucherLedger } from "@/lib/station/voucher-ledger"

export interface VoucherLineView {
  productId: string
  title: string
  productImage?: string | null
  quantity: number
}

export interface VoucherChoiceGroup {
  choiceGroup: number
  options: VoucherLineView[]
}

export interface VoucherChoiceSelection {
  choiceGroup: number
  productId: string
}

/**
 * Splits a slip into what is always handed out and the groups the operator still
 * has to choose from. Mirrors the server's rules so the stand can ask offline.
 */
export function resolveVoucherSelection(
  lines: VoucherLineView[],
  groupOf: Map<string, number>,
  selections: VoucherChoiceSelection[]
): { resolved: VoucherLineView[]; pending: VoucherChoiceGroup[] } {
  const groups = new Map<number, VoucherLineView[]>()
  const resolved: VoucherLineView[] = []

  for (const line of lines) {
    const group = groupOf.get(line.productId) ?? 0
    if (group <= 0) {
      resolved.push(line)
      continue
    }
    const bucket = groups.get(group)
    if (bucket) bucket.push(line)
    else groups.set(group, [line])
  }

  const picked = new Map(selections.map((sel) => [sel.choiceGroup, sel.productId]))
  const pending: VoucherChoiceGroup[] = []

  for (const group of Array.from(groups.keys()).sort((a, b) => a - b)) {
    const options = groups.get(group)!
    const choice = picked.get(group)
    if (choice === undefined) {
      if (options.length === 1) resolved.push(options[0]!)
      else pending.push({ choiceGroup: group, options })
      continue
    }
    const match = options.find((o) => o.productId === choice)
    if (match) resolved.push(match)
    else pending.push({ choiceGroup: group, options })
  }

  return { resolved, pending }
}

export type VoucherRedeemResult =
  | { kind: "invalid"; reason: VerifyFailure | "not-ready" }
  | { kind: "wrong-event" }
  | { kind: "scheduled"; validFrom: number }
  | { kind: "expired"; validUntil: number }
  | { kind: "revoked" }
  | { kind: "not-for-stand"; lines: VoucherLineView[] }
  | { kind: "already-redeemed"; redeemedAt: number; lines: VoucherLineView[] }
  | { kind: "sold-out"; message: string; lines: VoucherLineView[] }
  | { kind: "refused"; message: string; lines: VoucherLineView[] }
  | { kind: "auth-failed"; lines: VoucherLineView[] }
  /** The stand must pick one product per group before anything is handed out. */
  | { kind: "choice-required"; groups: VoucherChoiceGroup[]; lines: VoucherLineView[] }
  | { kind: "redeemed"; lines: VoucherLineView[]; pending: boolean }

export type ServerRedeemResult =
  | { ok: true; orderId: string; alreadyRedeemed: boolean }
  | { ok: false; reason: "auth" }
  | { ok: false; reason: "refused"; soldOut?: boolean; message?: string }
  | { ok: false; reason: "unreachable" }

export interface VoucherRedeemDeps {
  verify: (token: string) => { ok: true; payload: EntitlementPayload } | { ok: false; reason: VerifyFailure }
  stationTenantId: string | null
  assignedProductIds: Set<string>
  catalog: Map<string, CatalogProduct>
  ledger: VoucherLedger
  revoked: (voucherId: string) => boolean
  redeemOnServer: (voucherId: string, selections: VoucherChoiceSelection[]) => Promise<ServerRedeemResult>
  now: () => number
  /** Answers already given for this slip's choice groups. */
  selections?: VoucherChoiceSelection[]
}

function toViews(payload: EntitlementPayload, catalog: Map<string, CatalogProduct>): VoucherLineView[] {
  return payload.lines.map((line) => {
    const product = catalog.get(line.productId)
    return {
      productId: line.productId,
      title: product?.name ?? line.productId,
      productImage: product?.image ?? null,
      quantity: line.quantity,
    }
  })
}

export async function redeemVoucherScan(token: string, deps: VoucherRedeemDeps): Promise<VoucherRedeemResult> {
  const verified = deps.verify(token)
  if (!verified.ok) return { kind: "invalid", reason: verified.reason }

  const payload = verified.payload
  if (!deps.stationTenantId || payload.tenantId !== deps.stationTenantId) return { kind: "wrong-event" }

  const lines = toViews(payload, deps.catalog)

  if (deps.revoked(payload.voucherId)) return { kind: "revoked" }

  const nowSec = Math.floor(deps.now() / 1000)
  if (nowSec < payload.validFrom) return { kind: "scheduled", validFrom: payload.validFrom }
  if (nowSec >= payload.validUntil) return { kind: "expired", validUntil: payload.validUntil }

  const servesAll = payload.lines.every((line) => deps.assignedProductIds.has(line.productId))
  if (!servesAll) return { kind: "not-for-stand", lines }

  const groupOf = new Map(payload.lines.map((l) => [l.productId, l.choiceGroup]))
  const { resolved, pending } = resolveVoucherSelection(lines, groupOf, deps.selections ?? [])
  // Asked before the claim: an unanswered slip must stay untouched so the
  // operator can cancel the picker without burning it.
  if (pending.length > 0) return { kind: "choice-required", groups: pending, lines }

  const existing = await deps.ledger.get(payload.voucherId)
  if (existing) return { kind: "already-redeemed", redeemedAt: existing.redeemedAt, lines }

  const redeemedAt = deps.now()
  const outcome = await deps.ledger.claim({
    voucherId: payload.voucherId,
    tenantId: payload.tenantId,
    label: payload.label,
    lines: resolved.map((l) => ({ productId: l.productId, quantity: l.quantity })),
    redeemedAt,
    attempts: 0,
  })
  if (outcome === "duplicate") {
    const row = await deps.ledger.get(payload.voucherId)
    return { kind: "already-redeemed", redeemedAt: row?.redeemedAt ?? redeemedAt, lines }
  }

  const server = await deps.redeemOnServer(payload.voucherId, deps.selections ?? [])
  if (server.ok) {
    await deps.ledger.markSynced(payload.voucherId, server.orderId, deps.now())
    // The server owns the cross-device answer: this slip was already spent at
    // another stand, and handing out a second meal is the one thing single-use
    // must prevent.
    if (server.alreadyRedeemed) return { kind: "already-redeemed", redeemedAt: redeemedAt, lines }
    return { kind: "redeemed", lines: resolved, pending: false }
  }
  // Nothing was handed out yet, so release the claim rather than burning the
  // slip on a station that cannot talk to the server.
  if (server.reason === "auth") {
    await deps.ledger.remove(payload.voucherId)
    return { kind: "auth-failed", lines }
  }
  if (server.reason === "refused") {
    await deps.ledger.remove(payload.voucherId)
    if (server.soldOut) {
      return { kind: "sold-out", message: server.message ?? "Produkt ausverkauft", lines }
    }
    if (server.message) return { kind: "refused", message: server.message, lines }
    return { kind: "revoked" }
  }

  return { kind: "redeemed", lines: resolved, pending: true }
}
