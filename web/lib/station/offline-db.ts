/**
 * Station offline key-value store.
 *
 * The signed-QR public key and the lightweight catalog must survive service
 * worker upgrades — the SW wipes Cache Storage on every version bump — so they
 * live in IndexedDB, which the SW never touches. Product images go to the SW
 * cache instead (see the precache ritual), so this store stays small JSON.
 */

const DB_NAME = "comanda-station"
const DB_VERSION = 1
const KV_STORE = "kv"

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

function getDB(): Promise<IDBDatabase> {
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

function tx<T>(mode: IDBTransactionMode, run: (store: IDBObjectStore) => IDBRequest): Promise<T> {
  return getDB().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const transaction = db.transaction(KV_STORE, mode)
        const request = run(transaction.objectStore(KV_STORE))
        transaction.oncomplete = () => resolve(request.result as T)
        transaction.onerror = () => reject(transaction.error)
      })
  )
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
