/**
 * Server-side artist-voucher revocations, cached in IndexedDB so a fully-offline
 * station still refuses a voucher an admin revoked. A successful feed pull
 * REPLACES the cached set; a missing token / offline / any non-2xx is a no-op that
 * returns null, so the last-known set survives until the server is actually reached.
 */

import { idbKVStore, type StationKVStore } from "@/lib/station/offline-db"

export const REVOCATIONS_KEY = "revocations"

const REVOCATIONS_ENDPOINT = "/api/v1/plugins/artist-voucher/revocations"

export interface ServerRevocations {
  ids: string[]
  /** Device epoch ms of the last successful feed pull; null until we reach it. */
  syncedAt: number | null
}

export const EMPTY_REVOCATIONS: ServerRevocations = { ids: [], syncedAt: null }

export function revokedIdSet(state: ServerRevocations): Set<string> {
  return new Set(state.ids)
}

function formatSyncAge(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  if (s < 60) return `${s} Sek.`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} Min.`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} Std.`
  return `${Math.floor(h / 24)} Tg.`
}

export function syncedLabel(syncedAt: number | null, now: number): string {
  if (syncedAt == null) return "Noch nicht synchronisiert"
  return `Zuletzt synchronisiert vor ${formatSyncAge(now - syncedAt)}`
}

export async function loadRevocations(store: StationKVStore = idbKVStore()): Promise<ServerRevocations> {
  const server = await store.get<ServerRevocations>(REVOCATIONS_KEY).catch(() => null)
  return server ?? EMPTY_REVOCATIONS
}

export interface ReconcileDeps {
  fetchFn: typeof fetch
  token: () => string | null
  store?: StationKVStore
  now: () => number
  endpoint?: string
}

/**
 * REPLACE the cached revocation set from the feed. A missing token / offline / any
 * non-2xx is a no-op returning null, so the cache survives until the server is
 * actually reached. Caller passes a plain fetch (not the device-fetch wrapper): a
 * plugin-off 403 here must not clear the device token.
 */
export async function reconcileRevocations(deps: ReconcileDeps): Promise<ServerRevocations | null> {
  const token = deps.token()
  if (!token) return null
  const store = deps.store ?? idbKVStore()

  let res: Response
  try {
    res = await deps.fetchFn(deps.endpoint ?? REVOCATIONS_ENDPOINT, {
      headers: { Authorization: `Bearer ${token}` },
    })
  } catch {
    return null
  }
  if (!res.ok) return null

  const body = (await res.json().catch(() => null)) as { revokedIds?: unknown } | null
  if (!body || !Array.isArray(body.revokedIds)) return null
  const ids = body.revokedIds.filter((x): x is string => typeof x === "string")

  const server: ServerRevocations = { ids, syncedAt: deps.now() }
  await store.set(REVOCATIONS_KEY, server)
  return server
}
