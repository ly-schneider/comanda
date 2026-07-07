import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { OrderQueueManager } from "./order-queue"

describe("OrderQueueManager plugin payment", () => {
  beforeEach(() => {
    sessionStorage.clear()
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {}))
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("persists the plugin payload on the queued order", () => {
    const manager = new OrderQueueManager("tok", "dev")
    const order = manager.enqueue(
      [{ productId: "p1", quantity: 1 }],
      0,
      "gratis_plugin",
      undefined,
      undefined,
      undefined,
      { key: "club100", data: { personId: "x" } }
    )
    expect(order.paymentMethod).toBe("gratis_plugin")
    expect(order.plugin).toEqual({ key: "club100", data: { personId: "x" } })
    manager.destroy()
  })

  it("leaves the plugin payload unset for a plain payment", () => {
    const manager = new OrderQueueManager("tok", "dev")
    const order = manager.enqueue([{ productId: "p1", quantity: 1 }], 500, "cash")
    expect(order.plugin).toBeUndefined()
    manager.destroy()
  })
})
