import { backoffMs } from "@/lib/station/backoff"
import type { VoucherLedger, VoucherRedemption } from "@/lib/station/voucher-ledger"

export function voucherRedeemEndpoint(voucherId: string): string {
  return `/api/v1/plugins/artist-voucher/vouchers/${encodeURIComponent(voucherId)}/redeem`
}

export interface VoucherFlushDeps {
  ledger: VoucherLedger
  fetch: typeof fetch
  token: () => string | null
  now: () => number
  endpoint?: (voucherId: string) => string
}

export interface VoucherFlushSummary {
  synced: number
  failed: number
  skipped: number
  /** Refused for good by the server; the goods were already handed over. */
  abandoned: number
  remaining: number
}

function isTerminal(status: number): boolean {
  return status >= 400 && status < 500 && status !== 401 && status !== 403 && status !== 408 && status !== 429
}

let inFlight: Promise<VoucherFlushSummary> | null = null

export function flushVoucherRedemptions(deps: VoucherFlushDeps): Promise<VoucherFlushSummary> {
  if (inFlight) return inFlight
  inFlight = run(deps).finally(() => {
    inFlight = null
  })
  return inFlight
}

async function run(deps: VoucherFlushDeps): Promise<VoucherFlushSummary> {
  const endpoint = deps.endpoint ?? voucherRedeemEndpoint
  const items = await deps.ledger.pending()
  let synced = 0
  let failed = 0
  let skipped = 0
  let abandoned = 0

  const token = deps.token()
  if (!token) {
    return { synced: 0, failed: 0, skipped: items.length, abandoned: 0, remaining: items.length }
  }

  let authRejected = false

  for (const item of items) {
    const now = deps.now()
    if (item.nextAttemptAt != null && item.nextAttemptAt > now) {
      skipped++
      continue
    }
    if (authRejected) {
      failed++
      continue
    }

    const outcome = await upload(deps, endpoint(item.voucherId), token, item)
    if (outcome.kind === "ok") {
      await deps.ledger.markSynced(item.voucherId, outcome.orderId, deps.now())
      synced++
    } else if (outcome.kind === "auth") {
      authRejected = true
      failed++
    } else if (outcome.kind === "terminal") {
      // Unlike the interactive path, the food for this row is already in
      // someone's hands. Deleting it would lose the redemption AND hand the slip
      // back for a second meal, so it is kept and flagged instead.
      await deps.ledger.markFailed(item.voucherId, outcome.reason, deps.now())
      abandoned++
    } else {
      const attempts = item.attempts + 1
      await deps.ledger.put({
        ...item,
        attempts,
        lastAttemptAt: now,
        nextAttemptAt: now + backoffMs(attempts),
      })
      failed++
    }
  }

  return { synced, failed, skipped, abandoned, remaining: failed + skipped }
}

type UploadOutcome =
  { kind: "ok"; orderId: string } | { kind: "auth" } | { kind: "terminal"; reason: string } | { kind: "transient" }

async function upload(
  deps: VoucherFlushDeps,
  url: string,
  token: string,
  item: Pick<VoucherRedemption, "voucherId" | "selections">
): Promise<UploadOutcome> {
  let res: Response
  try {
    res = await deps.fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
        "Idempotency-Key": item.voucherId,
      },
      body: JSON.stringify({ selections: item.selections ?? [] }),
    })
  } catch {
    return { kind: "transient" }
  }

  if (res.status === 401 || res.status === 403) return { kind: "auth" }
  if (res.ok) {
    const body = (await res.json().catch(() => null)) as { orderId?: string } | null
    return body?.orderId ? { kind: "ok", orderId: body.orderId } : { kind: "transient" }
  }
  if (!isTerminal(res.status)) return { kind: "transient" }
  const body = (await res.json().catch(() => null)) as { code?: string } | null
  return { kind: "terminal", reason: body?.code ?? `http_${res.status}` }
}
