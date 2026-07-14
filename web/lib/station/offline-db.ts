/**
 * Station offline IndexedDB: a small key-value store, the redemption ledger,
 * and the server-sync outbox.
 *
 * Everything here must survive service worker upgrades — the SW wipes Cache
 * Storage on every version bump — so it lives in IndexedDB, which the SW never
 * touches. The KV store holds the signed-QR public key and the lightweight
 * catalog; the ledger holds the append-only (orderId, productId, variantId)
 * redemptions; the outbox holds redemptions still pending an idempotent server
 * write. Product image bytes go to the SW cache instead (see the precache ritual).
 */

const DB_NAME = "comanda-station"
const DB_VERSION = 4
const KV_STORE = "kv"
export const LEDGER_STORE = "ledger"
export const OUTBOX_STORE = "outbox"

export interface StationKVStore {
  get<T>(key: string): Promise<T | null>
  set<T>(key: string, value: T): Promise<void>
  delete(key: string): Promise<void>
  clear(): Promise<void>
}

function hasIndexedDB(): boolean {
  return typeof indexedDB !== "undefined"
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = (event) => {
      const db = req.result
      if (!db.objectStoreNames.contains(KV_STORE)) {
        db.createObjectStore(KV_STORE)
      }
      // A compound key path can't be altered in place, so the v4 variantId
      // addition drops and recreates the ledger store. Scoped to this migration
      // so a later bump can't re-wipe it; losing queued ledger rows here is
      // acceptable because the outbox is the durable server-sync path.
      if (event.oldVersion < 4) {
        if (db.objectStoreNames.contains(LEDGER_STORE)) db.deleteObjectStore(LEDGER_STORE)
        db.createObjectStore(LEDGER_STORE, { keyPath: ["orderId", "productId", "variantId"] })
      }
      if (!db.objectStoreNames.contains(OUTBOX_STORE)) {
        // Keyed by orderId: the server redeems every station-assigned line of an
        // order in one idempotent call, so there is at most one outbox row per
        // order and re-enqueueing the same order coalesces rather than duplicates.
        db.createObjectStore(OUTBOX_STORE, { keyPath: "orderId" })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

// One connection, reused across operations — a setup run does several reads and
// writes, and reopening the database for each one is pure overhead. The handle
// is dropped if the connection closes or another tab upgrades it, so the next
// call re-opens cleanly.
let dbPromise: Promise<IDBDatabase> | null = null

export function getStationDB(): Promise<IDBDatabase> {
  if (!dbPromise) {
    dbPromise = openDB()
      .then((db) => {
        db.onclose = () => {
          dbPromise = null
        }
        db.onversionchange = () => {
          db.close()
          dbPromise = null
        }
        return db
      })
      .catch((err) => {
        dbPromise = null
        throw err
      })
  }
  return dbPromise
}

export function stationHasIndexedDB(): boolean {
  return hasIndexedDB()
}

/**
 * Runs `run` inside a single-store transaction and resolves with the request
 * result on commit. The store name is explicit so both the KV store and the
 * ledger reuse the same connection and version.
 */
export function runStationTx<T>(
  storeName: string,
  mode: IDBTransactionMode,
  run: (store: IDBObjectStore) => IDBRequest
): Promise<T> {
  return getStationDB().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const transaction = db.transaction(storeName, mode)
        const request = run(transaction.objectStore(storeName))
        transaction.oncomplete = () => resolve(request.result as T)
        transaction.onerror = () => reject(transaction.error)
      })
  )
}

function tx<T>(mode: IDBTransactionMode, run: (store: IDBObjectStore) => IDBRequest): Promise<T> {
  return runStationTx<T>(KV_STORE, mode, run)
}

/**
 * The real IndexedDB-backed store. On platforms without IndexedDB (SSR) every
 * operation degrades to a no-op rather than throwing, so callers don't need to
 * guard each call.
 */
export function idbKVStore(): StationKVStore {
  return {
    async get<T>(key: string): Promise<T | null> {
      if (!hasIndexedDB()) return null
      const value = await tx<T | undefined>("readonly", (s) => s.get(key))
      return value ?? null
    },
    async set<T>(key: string, value: T): Promise<void> {
      if (!hasIndexedDB()) return
      await tx<IDBValidKey>("readwrite", (s) => s.put(value, key))
    },
    async delete(key: string): Promise<void> {
      if (!hasIndexedDB()) return
      await tx<undefined>("readwrite", (s) => s.delete(key))
    },
    async clear(): Promise<void> {
      if (!hasIndexedDB()) return
      await tx<undefined>("readwrite", (s) => s.clear())
    },
  }
}

/** In-memory store with the same contract; used by tests and SSR fallbacks. */
export function memoryKVStore(seed?: Record<string, unknown>): StationKVStore {
  const map = new Map<string, unknown>(seed ? Object.entries(seed) : [])
  return {
    async get<T>(key: string): Promise<T | null> {
      return map.has(key) ? (map.get(key) as T) : null
    },
    async set<T>(key: string, value: T): Promise<void> {
      map.set(key, value)
    },
    async delete(key: string): Promise<void> {
      map.delete(key)
    },
    async clear(): Promise<void> {
      map.clear()
    },
  }
}
