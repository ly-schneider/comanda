import "fake-indexeddb/auto"
import { beforeEach, describe, expect, it } from "vitest"
import { idbOutbox, memoryOutbox, type StationOutbox } from "./sync-queue"

// Both implementations satisfy the same contract; run the suite against each.
function contract(name: string, make: () => StationOutbox) {
  describe(name, () => {
    let outbox: StationOutbox
    beforeEach(async () => {
      outbox = make()
      await outbox.clear()
    })

    it("enqueues a fresh item with zeroed retry state", async () => {
      await outbox.enqueue({ orderId: "o1", tenantId: "tenant_seed1", productIds: ["pA"] }, 1000)
      const items = await outbox.list()
      expect(items).toHaveLength(1)
      expect(items[0]).toMatchObject({
        orderId: "o1",
        tenantId: "tenant_seed1",
        productIds: ["pA"],
        createdAt: 1000,
        attempts: 0,
      })
    })

    it("coalesces a repeat enqueue: unions products, keeps createdAt and retry state", async () => {
      await outbox.enqueue({ orderId: "o1", tenantId: "tenant_seed1", productIds: ["pA"] }, 1000)
      await outbox.put({
        orderId: "o1",
        tenantId: "tenant_seed1",
        productIds: ["pA"],
        createdAt: 1000,
        attempts: 2,
        lastAttemptAt: 5000,
        nextAttemptAt: 9000,
      })

      await outbox.enqueue({ orderId: "o1", tenantId: "tenant_seed1", productIds: ["pA", "pB"] }, 7000)
      const items = await outbox.list()
      expect(items).toHaveLength(1)
      expect(items[0]!.productIds.sort()).toEqual(["pA", "pB"])
      // A re-enqueue must not reset backoff progress or the original timestamp.
      expect(items[0]).toMatchObject({ createdAt: 1000, attempts: 2, nextAttemptAt: 9000 })
    })

    it("put overwrites an item (used to record a failed attempt's backoff)", async () => {
      await outbox.enqueue({ orderId: "o1", tenantId: "tenant_seed1", productIds: ["pA"] }, 1000)
      await outbox.put({
        orderId: "o1",
        tenantId: "tenant_seed1",
        productIds: ["pA"],
        createdAt: 1000,
        attempts: 1,
        lastAttemptAt: 2000,
        nextAttemptAt: 4000,
      })
      const items = await outbox.list()
      expect(items[0]).toMatchObject({ attempts: 1, lastAttemptAt: 2000, nextAttemptAt: 4000 })
    })

    it("remove drops a synced item", async () => {
      await outbox.enqueue({ orderId: "o1", tenantId: "tenant_seed1", productIds: ["pA"] }, 1000)
      await outbox.enqueue({ orderId: "o2", tenantId: "tenant_seed1", productIds: ["pB"] }, 1000)
      await outbox.remove("o1")
      const items = await outbox.list()
      expect(items.map((i) => i.orderId)).toEqual(["o2"])
    })
  })
}

contract("memoryOutbox", memoryOutbox)
contract("idbOutbox", idbOutbox)

describe("idbOutbox persistence", () => {
  it("survives a fresh database connection (reload/restart)", async () => {
    const outbox = idbOutbox()
    await outbox.clear()
    await outbox.enqueue({ orderId: "order_persist", tenantId: "tenant_seed1", productIds: ["pX"] }, 4242)

    const fromDisk = await new Promise<unknown>((resolve, reject) => {
      const req = indexedDB.open("comanda-station", 3)
      req.onsuccess = () => {
        const db = req.result
        const tx = db.transaction("outbox", "readonly")
        const get = tx.objectStore("outbox").get("order_persist")
        get.onsuccess = () => {
          resolve(get.result)
          db.close()
        }
        get.onerror = () => reject(get.error)
      }
      req.onerror = () => reject(req.error)
    })

    expect(fromDisk).toMatchObject({ orderId: "order_persist", productIds: ["pX"], createdAt: 4242 })
  })
})
