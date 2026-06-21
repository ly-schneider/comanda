import "fake-indexeddb/auto"
import { beforeEach, describe, expect, it } from "vitest"
import { idbLedger, type LedgerEntry, memoryLedger, type StationLedger } from "./ledger"

function entry(orderId: string, productId: string, overrides: Partial<LedgerEntry> = {}): LedgerEntry {
  return { orderId, productId, quantity: 1, tenantId: "tenant_seed1", redeemedAt: 1000, ...overrides }
}

// Both implementations satisfy the same contract; run the suite against each.
function contract(name: string, make: () => StationLedger) {
  describe(name, () => {
    let ledger: StationLedger
    beforeEach(async () => {
      ledger = make()
      await ledger.clear()
    })

    it("records a new redemption and reports it present", async () => {
      expect(await ledger.has("o1", "pA")).toBe(false)
      expect(await ledger.record(entry("o1", "pA"))).toBe("recorded")
      expect(await ledger.has("o1", "pA")).toBe(true)
      expect(await ledger.get("o1", "pA")).toMatchObject({ orderId: "o1", productId: "pA", quantity: 1 })
    })

    it("is append-only: a duplicate (orderId, productId) does not overwrite", async () => {
      await ledger.record(entry("o1", "pA", { redeemedAt: 1000 }))
      const outcome = await ledger.record(entry("o1", "pA", { redeemedAt: 9999, quantity: 5 }))
      expect(outcome).toBe("duplicate")
      const stored = await ledger.get("o1", "pA")
      expect(stored?.redeemedAt).toBe(1000)
      expect(stored?.quantity).toBe(1)
      expect(await ledger.list()).toHaveLength(1)
    })

    it("keys per product, so the same order's other products stay redeemable", async () => {
      await ledger.record(entry("o1", "pA"))
      expect(await ledger.has("o1", "pA")).toBe(true)
      expect(await ledger.has("o1", "pB")).toBe(false)
      expect(await ledger.record(entry("o1", "pB"))).toBe("recorded")
      expect(await ledger.list()).toHaveLength(2)
    })

    it("keys per order, so the same product in another order is independent", async () => {
      await ledger.record(entry("o1", "pA"))
      expect(await ledger.has("o2", "pA")).toBe(false)
      expect(await ledger.record(entry("o2", "pA"))).toBe("recorded")
    })
  })
}

contract("memoryLedger", memoryLedger)
contract("idbLedger", idbLedger)

describe("idbLedger persistence", () => {
  it("survives a fresh database connection (reload/restart)", async () => {
    const ledger = idbLedger()
    await ledger.clear()
    await ledger.record(entry("order_persist", "prod_x", { redeemedAt: 4242 }))

    // Open an independent connection, bypassing the module's cached handle, to
    // prove the entry is durably in IndexedDB and not just in process memory.
    const fromDisk = await new Promise<LedgerEntry | undefined>((resolve, reject) => {
      const req = indexedDB.open("comanda-station", 2)
      req.onsuccess = () => {
        const db = req.result
        const tx = db.transaction("ledger", "readonly")
        const get = tx.objectStore("ledger").get(["order_persist", "prod_x"])
        get.onsuccess = () => {
          resolve(get.result as LedgerEntry | undefined)
          db.close()
        }
        get.onerror = () => reject(get.error)
      }
      req.onerror = () => reject(req.error)
    })

    expect(fromDisk).toMatchObject({ orderId: "order_persist", productId: "prod_x", redeemedAt: 4242 })
  })
})
