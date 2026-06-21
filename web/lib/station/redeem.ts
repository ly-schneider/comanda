/**
 * The single offline-first redemption decision.
 *
 * Verification and the local ledger are always the on-device source of truth:
 * decode + Ed25519-verify, enforce tenant scoping, match this station's
 * assigned products, dedup against the append-only ledger, then write it. The
 * server write is the only online/offline difference — online persists
 * immediately; offline the ledger stands alone until a later sync flush.
 *
 * Pure orchestration with injected effects (verify, ledger, persist) so every
 * rejection branch is unit-testable without a browser.
 */

import type { StationLedger } from "@/lib/station/ledger"
import type { VerifyResult } from "@/lib/station/qr-verify"

export interface RedeemLineView {
  productId: string
  title: string
  productImage?: string | null
  quantity: number
  /** epoch ms; set once the line is redeemed (now or on a prior scan) */
  redeemedAt?: number
}

export type RedeemResult =
  | { kind: "invalid" } // bad signature / malformed / unknown version
  | { kind: "wrong-event" } // tenant id ≠ this station's tenant
  | { kind: "not-for-stand" } // no payload line for an assigned product
  | { kind: "already-redeemed"; orderId: string; lines: RedeemLineView[] }
  | { kind: "redeemed"; orderId: string; lines: RedeemLineView[] }

export interface CatalogProduct {
  name: string
  image?: string | null
}

export interface RedeemDeps {
  verify: (token: string) => VerifyResult
  stationTenantId: string | null
  assignedProductIds: Set<string>
  catalog: Map<string, CatalogProduct>
  ledger: StationLedger
  online: boolean
  /** Online direct-save; the offline sync flush is wired separately. */
  persist: (orderId: string, productIds: string[]) => void
  now: () => number
}

export async function redeemScan(token: string, deps: RedeemDeps): Promise<RedeemResult> {
  const verified = deps.verify(token)
  if (!verified.ok) return { kind: "invalid" }

  const payload = verified.payload
  // A valid signature only proves *some* tenant signed it. On the fully-offline
  // path there is no server backstop, so the station is the sole enforcer of
  // tenant scoping — reject anything not addressed to our tenant.
  if (!deps.stationTenantId || payload.t !== deps.stationTenantId) return { kind: "wrong-event" }

  const matched = payload.l.filter((line) => deps.assignedProductIds.has(line.p))
  if (matched.length === 0) return { kind: "not-for-stand" }

  const views: RedeemLineView[] = []
  const fresh: typeof matched = []
  for (const line of matched) {
    const existing = await deps.ledger.get(payload.o, line.p)
    const product = deps.catalog.get(line.p)
    views.push({
      productId: line.p,
      title: product?.name ?? line.p,
      productImage: product?.image ?? null,
      quantity: line.q,
      redeemedAt: existing?.redeemedAt,
    })
    if (!existing) fresh.push(line)
  }

  if (fresh.length === 0) {
    return { kind: "already-redeemed", orderId: payload.o, lines: views }
  }

  const redeemedAt = deps.now()
  for (const line of fresh) {
    await deps.ledger.record({
      orderId: payload.o,
      productId: line.p,
      quantity: line.q,
      tenantId: payload.t,
      redeemedAt,
    })
  }

  if (deps.online)
    deps.persist(
      payload.o,
      fresh.map((l) => l.p)
    )

  const lines = views.map((v) => (v.redeemedAt != null ? v : { ...v, redeemedAt }))
  return { kind: "redeemed", orderId: payload.o, lines }
}
