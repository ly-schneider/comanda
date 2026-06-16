/**
 * Offline-first station identity: online it comes from `/stations/me`, offline
 * it is replayed from the copy cached during the setup ritual. The resolver
 * makes no network request while offline.
 */

import { isStationTokenUsableOffline } from "@/lib/device-auth"
import { idbKVStore, type StationKVStore } from "@/lib/station/offline-db"

export const STATION_SESSION_KEY = "stationSession"
export const STATION_ME_URL = "/api/v1/stations/me"

export type StationApprovalStatus = "pending" | "approved" | "rejected" | "revoked"

export interface StationProduct {
  productId: string
  name?: string
}

/** Mirrors the authenticated `/stations/me` payload. */
export interface StationStatus {
  id: string
  status: StationApprovalStatus
  name?: string
  products?: StationProduct[]
}

export interface CachedStationSession {
  status: StationStatus
  cachedAt: number
}

export function getCachedStationSession(store: StationKVStore = idbKVStore()): Promise<CachedStationSession | null> {
  return store.get<CachedStationSession>(STATION_SESSION_KEY)
}

export function cacheStationSession(
  status: StationStatus,
  now = Date.now(),
  store: StationKVStore = idbKVStore()
): Promise<void> {
  return store.set<CachedStationSession>(STATION_SESSION_KEY, { status, cachedAt: now })
}

export function clearCachedStationSession(store: StationKVStore = idbKVStore()): Promise<void> {
  return store.delete(STATION_SESSION_KEY)
}

export type SessionResolution =
  | { kind: "no-token" }
  | { kind: "system-disabled" }
  | { kind: "unauthorized" }
  | { kind: "offline-unprepared" }
  | { kind: "resolved"; status: StationStatus; source: "network" | "cache" }

export interface ResolveSessionDeps {
  token: string | null
  online: boolean
  fetchFn: typeof fetch
  store: StationKVStore
  now: () => number
}

// Offline: resolve from cache without any network request. Online: the server
// is authoritative, but an unreachable server falls back to the cached identity.
export async function resolveStationSession(deps: ResolveSessionDeps): Promise<SessionResolution> {
  const { token, online, fetchFn, store } = deps

  if (!isStationTokenUsableOffline(token)) return { kind: "no-token" }

  const fromCache = async (): Promise<SessionResolution> => {
    const cached = await getCachedStationSession(store).catch(() => null)
    return cached ? { kind: "resolved", status: cached.status, source: "cache" } : { kind: "offline-unprepared" }
  }

  if (!online) return fromCache()

  let res: Response
  try {
    res = await fetchFn(STATION_ME_URL, { headers: { Authorization: `Bearer ${token}` } })
  } catch {
    // Server unreachable (network partition): replay the cached identity so
    // redemption isn't blocked on connectivity.
    return fromCache()
  }

  if (res.status === 503) return { kind: "system-disabled" }
  if (res.status === 401 || res.status === 403) {
    await clearCachedStationSession(store).catch(() => {})
    return { kind: "unauthorized" }
  }
  // The server answered but not authoritatively (5xx, an unexpected status, or
  // an unparseable body). Don't mint a fresh identity from it — degrade to the
  // cached one rather than masking the error as a clean resolve. Only an
  // explicit 401/403 above ever revokes the cached identity.
  if (!res.ok) return fromCache()

  let status: StationStatus
  try {
    status = (await res.json()) as StationStatus
  } catch {
    return fromCache()
  }
  await cacheStationSession(status, deps.now(), store).catch(() => {})
  return { kind: "resolved", status, source: "network" }
}
