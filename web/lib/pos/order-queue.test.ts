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
      { key: "demo", data: { personId: "x" } }
    )
    expect(order.paymentMethod).toBe("gratis_plugin")
    expect(order.plugin).toEqual({ key: "demo", data: { personId: "x" } })
    manager.destroy()
  })

  it("leaves the plugin payload unset for a plain payment", () => {
    const manager = new OrderQueueManager("tok", "dev")
    const order = manager.enqueue([{ productId: "p1", quantity: 1 }], 500, "cash")
    expect(order.plugin).toBeUndefined()
    manager.destroy()
  })
})

describe("OrderQueueManager online create + pay-only", () => {
  beforeEach(() => sessionStorage.clear())
  afterEach(() => vi.unstubAllGlobals())

  it("createOrderNow surfaces out_of_stock with the sold-out items", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({ code: "out_of_stock", details: { items: [{ productId: "p1", available: 0 }] } }),
            {
              status: 409,
            }
          )
        )
      )
    )
    const manager = new OrderQueueManager("tok", "dev")
    const res = await manager.createOrderNow([{ productId: "p1", quantity: 5 }])
    expect(res.ok).toBe(false)
    if (res.ok || res.code !== "out_of_stock") throw new Error("expected out_of_stock")
    expect(res.soldOut).toEqual([{ productId: "p1", available: 0 }])
    manager.destroy()
  })

  it("createOrderNow returns the server id + qr on success", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(new Response(JSON.stringify({ id: "srv9", qrPayload: "qr9" }), { status: 201 })))
    )
    const manager = new OrderQueueManager("tok", "dev")
    const res = await manager.createOrderNow([{ productId: "p1", quantity: 1 }])
    expect(res).toMatchObject({ ok: true, serverId: "srv9", serverQrPayload: "qr9" })
    manager.destroy()
  })

  it("a pre-created order skips create and only runs the pay step", async () => {
    const calls: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, opts?: { method?: string }) => {
        calls.push(`${opts?.method || "GET"} ${url}`)
        if (String(url).endsWith("/payment")) {
          return Promise.resolve(new Response(JSON.stringify({ id: "pay1" }), { status: 201 }))
        }
        return Promise.resolve(new Response(JSON.stringify({}), { status: 500 }))
      })
    )
    const manager = new OrderQueueManager("tok", "dev")
    const paid = new Promise<void>((resolve) => {
      manager.onSync((o) => {
        if (o.status === "paid") resolve()
      })
    })
    manager.enqueue([{ productId: "p1", quantity: 1 }], 500, "cash", undefined, undefined, undefined, undefined, {
      serverId: "srv1",
      idempotencyKey: "idem1",
    })
    await paid
    expect(calls).toContain("POST /api/v1/orders/srv1/payment")
    expect(calls).not.toContain("POST /api/v1/orders")
    manager.destroy()
  })
})
