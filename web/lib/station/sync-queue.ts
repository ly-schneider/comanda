/**
 * The server-sync outbox — pending idempotent redemption uploads.
 *
 * The ledger (see `ledger.ts`) is the on-device dedup authority and is written
 * first; the outbox is the best-effort mirror of those redemptions to the
 * server. An item present here means "not yet acknowledged by the server"; a
 * successful flush removes it (see `sync.ts`). Keyed by orderId because the
 * server redeems every station-assigned line of an order in one idempotent
 * call, so one row per order is enough and re-enqueueing coalesces.
 *
 * It lives in IndexedDB so a queued redemption survives reload, restart, and
 * service-worker upgrades until connectivity returns.
 */

import { getStationDB, OUTBOX_STORE, runStationTx, stationHasIndexedDB } from "@/lib/station/offline-db"

export interface OutboxItem {
  orderId: string
  tenantId: string
  /** Products redeemed for this order at this station; informational/reporting. */
  productIds: string[]
  /** epoch ms first enqueued */
  createdAt: number
  /** failed upload attempts so far */
  attempts: number
  /** epoch ms of the last upload attempt */
  lastAttemptAt?: number
  /** epoch ms; the flush skips this item until now ≥ nextAttemptAt (backoff) */
  nextAttemptAt?: number
}

export interface OutboxEntry {
  orderId: string
  tenantId: string
  productIds: string[]
}

export interface StationOutbox {
  /**
   * Queues a redemption for upload. Idempotent: a second enqueue of the same
   * order unions the product ids onto the existing row without resetting its
   * retry state, so it never duplicates or loses backoff progress.
   */
  enqueue(entry: OutboxEntry, now: number): Promise<void>
  list(): Promise<OutboxItem[]>
  /** Overwrites a row — used to record a failed attempt's backoff state. */
  put(item: OutboxItem): Promise<void>
  /** Removes a row once the server has acknowledged it. */
  remove(orderId: string): Promise<void>
  clear(): Promise<void>
}

function merge(existing: OutboxItem | undefined, entry: OutboxEntry, now: number): OutboxItem {
  if (!existing) {
    return { ...entry, productIds: Array.from(new Set(entry.productIds)), createdAt: now, attempts: 0 }
  }
  return { ...existing, productIds: Array.from(new Set([...existing.productIds, ...entry.productIds])) }
}

export function idbOutbox(): StationOutbox {
  return {
    async enqueue(entry, now) {
      if (!stationHasIndexedDB()) return
      const db = await getStationDB()
      return new Promise<void>((resolve, reject) => {
        const tx = db.transaction(OUTBOX_STORE, "readwrite")
        const store = tx.objectStore(OUTBOX_STORE)
        // Read and write in one transaction so the coalesce is atomic.
        const get = store.get(entry.orderId)
        get.onsuccess = () => store.put(merge(get.result as OutboxItem | undefined, entry, now))
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error)
      })
    },
    async list() {
      if (!stationHasIndexedDB()) return []
      const all = await runStationTx<OutboxItem[]>(OUTBOX_STORE, "readonly", (s) => s.getAll())
      return all ?? []
    },
    async put(item) {
      if (!stationHasIndexedDB()) return
      await runStationTx<IDBValidKey>(OUTBOX_STORE, "readwrite", (s) => s.put(item))
    },
    async remove(orderId) {
      if (!stationHasIndexedDB()) return
      await runStationTx<undefined>(OUTBOX_STORE, "readwrite", (s) => s.delete(orderId))
    },
    async clear() {
      if (!stationHasIndexedDB()) return
      await runStationTx<undefined>(OUTBOX_STORE, "readwrite", (s) => s.clear())
    },
  }
}

/** In-memory outbox with the same contract; used by tests and SSR fallbacks. */
export function memoryOutbox(): StationOutbox {
  const map = new Map<string, OutboxItem>()
  return {
    async enqueue(entry, now) {
      map.set(entry.orderId, merge(map.get(entry.orderId), entry, now))
    },
    async list() {
      return Array.from(map.values())
    },
    async put(item) {
      map.set(item.orderId, item)
    },
    async remove(orderId) {
      map.delete(orderId)
    },
    async clear() {
      map.clear()
    },
  }
}
