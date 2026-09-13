import { getStationDB, runStationTx, stationHasIndexedDB, VOUCHER_STORE } from "@/lib/station/offline-db"

export interface VoucherRedemptionLine {
  productId: string
  quantity: number
}

export interface VoucherRedemption {
  voucherId: string
  tenantId: string
  label: string
  lines: VoucherRedemptionLine[]
  redeemedAt: number
  syncedAt?: number
  orderId?: string
  attempts: number
  lastAttemptAt?: number
  nextAttemptAt?: number
  /**
   * Set when the server refused the upload for good. The goods were already
   * handed over, so the row stays as this device's dedup record and stops
   * retrying; it is surfaced instead of silently dropped.
   */
  failedAt?: number
  failedReason?: string
}

export type ClaimOutcome = "claimed" | "duplicate"

export interface VoucherLedger {
  get(voucherId: string): Promise<VoucherRedemption | null>
  claim(entry: VoucherRedemption): Promise<ClaimOutcome>
  pending(): Promise<VoucherRedemption[]>
  markSynced(voucherId: string, orderId: string, at: number): Promise<void>
  markFailed(voucherId: string, reason: string, at: number): Promise<void>
  remove(voucherId: string): Promise<void>
  put(entry: VoucherRedemption): Promise<void>
  list(): Promise<VoucherRedemption[]>
  clear(): Promise<void>
}

export function idbVoucherLedger(): VoucherLedger {
  return {
    async get(voucherId) {
      if (!stationHasIndexedDB()) return null
      const entry = await runStationTx<VoucherRedemption | undefined>(VOUCHER_STORE, "readonly", (s) =>
        s.get(voucherId)
      )
      return entry ?? null
    },

    async claim(entry) {
      if (!stationHasIndexedDB()) return "claimed"
      const db = await getStationDB()
      return new Promise<ClaimOutcome>((resolve, reject) => {
        const tx = db.transaction(VOUCHER_STORE, "readwrite")
        const store = tx.objectStore(VOUCHER_STORE)
        let outcome: ClaimOutcome = "claimed"
        const existing = store.get(entry.voucherId)
        existing.onsuccess = () => {
          if (existing.result) outcome = "duplicate"
          else store.add(entry)
        }
        tx.oncomplete = () => resolve(outcome)
        tx.onerror = () => reject(tx.error)
        tx.onabort = () => reject(tx.error)
      })
    },

    async pending() {
      const all = await this.list()
      return all.filter((e) => e.syncedAt == null && e.failedAt == null)
    },

    async markSynced(voucherId, orderId, at) {
      if (!stationHasIndexedDB()) return
      const db = await getStationDB()
      return new Promise<void>((resolve, reject) => {
        const tx = db.transaction(VOUCHER_STORE, "readwrite")
        const store = tx.objectStore(VOUCHER_STORE)
        const existing = store.get(voucherId)
        existing.onsuccess = () => {
          const row = existing.result as VoucherRedemption | undefined
          if (row) store.put({ ...row, syncedAt: at, orderId })
        }
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error)
        tx.onabort = () => reject(tx.error)
      })
    },

    async markFailed(voucherId, reason, at) {
      if (!stationHasIndexedDB()) return
      const db = await getStationDB()
      return new Promise<void>((resolve, reject) => {
        const tx = db.transaction(VOUCHER_STORE, "readwrite")
        const store = tx.objectStore(VOUCHER_STORE)
        const existing = store.get(voucherId)
        existing.onsuccess = () => {
          const row = existing.result as VoucherRedemption | undefined
          if (row) store.put({ ...row, failedAt: at, failedReason: reason })
        }
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error)
        tx.onabort = () => reject(tx.error)
      })
    },

    async remove(voucherId) {
      if (!stationHasIndexedDB()) return
      await runStationTx<undefined>(VOUCHER_STORE, "readwrite", (s) => s.delete(voucherId))
    },

    async put(entry) {
      if (!stationHasIndexedDB()) return
      await runStationTx<IDBValidKey>(VOUCHER_STORE, "readwrite", (s) => s.put(entry))
    },

    async list() {
      if (!stationHasIndexedDB()) return []
      const rows = await runStationTx<VoucherRedemption[]>(VOUCHER_STORE, "readonly", (s) => s.getAll())
      return rows ?? []
    },

    async clear() {
      if (!stationHasIndexedDB()) return
      await runStationTx<undefined>(VOUCHER_STORE, "readwrite", (s) => s.clear())
    },
  }
}
