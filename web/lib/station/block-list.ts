/**
 * Offline block set = server revocation feed ∪ local manual blocks, in IndexedDB.
 * A successful feed pull REPLACES the server set and CLEARS the manual list —
 * server is the source of truth and local blocks are never uploaded.
 */

import { idbKVStore, type StationKVStore } from "@/lib/station/offline-db"
import { ENTITY_ID_RE } from "@/lib/station-scan"

export const REVOCATIONS_KEY = "revocations"
export const MANUAL_BLOCKS_KEY = "manualBlocks"

const REVOCATIONS_ENDPOINT = "/api/v1/plugins/artist-voucher/revocations"

export interface ServerRevocations {
  ids: string[]
  /** Device epoch ms of the last successful feed pull; null until we reach it. */
  syncedAt: number | null
}

export interface BlockState {
  server: ServerRevocations
  manual: string[]
}

const EMPTY_SERVER: ServerRevocations = { ids: [], syncedAt: null }

export const EMPTY_BLOCK_STATE: BlockState = { server: EMPTY_SERVER, manual: [] }

export function isValidBlockId(id: string): boolean {
  return ENTITY_ID_RE.test(id)
}

export function effectiveBlockSet(state: BlockState): Set<string> {
  return new Set([...state.server.ids, ...state.manual])
}

export async function loadBlockState(store: StationKVStore = idbKVStore()): Promise<BlockState> {
  const [server, manual] = await Promise.all([
    store.get<ServerRevocations>(REVOCATIONS_KEY).catch(() => null),
    store.get<string[]>(MANUAL_BLOCKS_KEY).catch(() => null),
  ])
  return { server: server ?? EMPTY_SERVER, manual: manual ?? [] }
}

// Nanoids are case-sensitive, so ids are stored verbatim — never normalized.
export async function addManualBlock(id: string, store: StationKVStore = idbKVStore()): Promise<string[]> {
  const current = (await store.get<string[]>(MANUAL_BLOCKS_KEY).catch(() => null)) ?? []
  if (current.includes(id)) return current
  const next = [...current, id]
  await store.set(MANUAL_BLOCKS_KEY, next)
  return next
}

export async function removeManualBlock(id: string, store: StationKVStore = idbKVStore()): Promise<string[]> {
  const current = (await store.get<string[]>(MANUAL_BLOCKS_KEY).catch(() => null)) ?? []
  if (!current.includes(id)) return current
  const next = current.filter((x) => x !== id)
  await store.set(MANUAL_BLOCKS_KEY, next)
  return next
}

export interface ReconcileDeps {
  fetchFn: typeof fetch
  token: () => string | null
  store?: StationKVStore
  now: () => number
  endpoint?: string
}

/**
 * REPLACE the cached server set and CLEAR the manual list from the feed. A missing
 * token / offline / any non-2xx is a no-op returning null, so manual blocks survive
 * until the server is actually reached. Caller passes a plain fetch (not the
 * device-fetch wrapper): a plugin-off 403 here must not clear the device token.
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
  await store.set(MANUAL_BLOCKS_KEY, [])
  return server
}
