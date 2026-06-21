/**
 * Draining the redemption outbox to the server.
 *
 * One idempotent ingest, two timings: a redemption is enqueued the moment it is
 * written to the ledger, then flushed here — immediately when online (the page
 * triggers a flush right after the scan, preserving the instant-save UX) or
 * opportunistically when connectivity returns (online event / periodic retry).
 *
 * The server endpoint is idempotent on the redemption identity, so a retried or
 * duplicate upload is a no-op. Each item carries a stable Idempotency-Key (its
 * orderId) so the server can short-circuit retries, and a failed upload is kept
 * with exponential backoff — never dropped — so no redemption is lost.
 */

import type { StationOutbox } from "@/lib/station/sync-queue"

const REDEEM_ENDPOINT = "/api/v1/stations/redeem"
const BASE_BACKOFF_MS = 2000
const MAX_BACKOFF_MS = 60_000

export interface FlushDeps {
  outbox: StationOutbox
  fetch: typeof fetch
  /** Long-lived device token; null when unavailable (flush is skipped). */
  token: () => string | null
  now: () => number
  endpoint?: string
}

export interface FlushSummary {
  synced: number
  failed: number
  /** Not yet due for retry (still in backoff). */
  skipped: number
  /** Items remaining in the outbox after this pass. */
  remaining: number
}

function backoffMs(attempts: number): number {
  return Math.min(BASE_BACKOFF_MS * 2 ** attempts, MAX_BACKOFF_MS)
}

let inFlight: Promise<FlushSummary> | null = null

/**
 * Flushes every due outbox item. Re-entrant calls (online event firing while a
 * periodic flush runs) share the in-flight pass instead of double-sending.
 */
export function flushOutbox(deps: FlushDeps): Promise<FlushSummary> {
  if (inFlight) return inFlight
  inFlight = run(deps).finally(() => {
    inFlight = null
  })
  return inFlight
}

async function run(deps: FlushDeps): Promise<FlushSummary> {
  const endpoint = deps.endpoint ?? REDEEM_ENDPOINT
  const items = await deps.outbox.list()
  let synced = 0
  let failed = 0
  let skipped = 0

  const token = deps.token()
  if (!token) {
    // No usable auth: leave everything queued rather than drop it.
    return { synced: 0, failed: 0, skipped: items.length, remaining: items.length }
  }

  // Once the device token is rejected, every remaining item would fail the same
  // way: stop hitting the server and leave them queued for the next flush (after
  // the operator re-pairs). The fetch wrapper handles clearing the token and
  // redirecting to setup; bumping backoff for an auth error would be pointless.
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

    let ok = false
    let authError = false
    try {
      const res = await deps.fetch(endpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
          "Idempotency-Key": item.orderId,
        },
        body: JSON.stringify({ orderId: item.orderId }),
      })
      ok = res.ok
      authError = res.status === 401 || res.status === 403
    } catch {
      ok = false
    }

    if (ok) {
      await deps.outbox.remove(item.orderId)
      synced++
    } else if (authError) {
      authRejected = true
      failed++
    } else {
      const attempts = item.attempts + 1
      await deps.outbox.put({
        ...item,
        attempts,
        lastAttemptAt: now,
        nextAttemptAt: now + backoffMs(attempts),
      })
      failed++
    }
  }

  return { synced, failed, skipped, remaining: failed + skipped }
}
