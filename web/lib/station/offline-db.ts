/**
 * Station offline IndexedDB: a small key-value store plus the redemption ledger.
 *
 * Everything here must survive service worker upgrades — the SW wipes Cache
 * Storage on every version bump — so it lives in IndexedDB, which the SW never
 * touches. The KV store holds the signed-QR public key and the lightweight
 * catalog; the ledger holds the append-only (orderId, productId) redemptions.
 * Product image bytes go to the SW cache instead (see the precache ritual).
 */

const DB_NAME = "comanda-station"
const DB_VERSION = 2
const KV_STORE = "kv"
export const LEDGER_STORE = "ledger"

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
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains(KV_STORE)) {
        db.createObjectStore(KV_STORE)
      }
      if (!db.objectStoreNames.contains(LEDGER_STORE)) {
        // Compound key (orderId, productId): the dedup identity of a redemption,
        // and what makes the store inherently append-once per item.
        db.createObjectStore(LEDGER_STORE, { keyPath: ["orderId", "productId"] })
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
