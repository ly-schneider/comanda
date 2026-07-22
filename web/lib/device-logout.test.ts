import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { memoryOutbox } from "@/lib/station/sync-queue"
import { logoutDevice, unsyncedWorkCount } from "./device-logout"

const DEVICE_TOKEN_KEY = "comanda.deviceToken"
const DEVICE_TYPE_KEY = "comanda.deviceType"
const POS_QUEUE_KEY = "comanda-pos-order-queue"

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe("logoutDevice", () => {
  it("revokes the station server-side, clears the token, wipes offline state, and redirects", async () => {
    localStorage.setItem(DEVICE_TOKEN_KEY, "tok_station")
    localStorage.setItem(DEVICE_TYPE_KEY, "station")
    const fetchFn = vi.fn().mockResolvedValue({ ok: true, status: 204 } as Response)
    const clearStationDb = vi.fn().mockResolvedValue(undefined)
    const clearCaches = vi.fn().mockResolvedValue(undefined)
    const redirect = vi.fn()

    await logoutDevice("station", { fetchFn, clearStationDb, clearCaches, redirect })

    expect(fetchFn).toHaveBeenCalledWith("/api/v1/stations/me", {
      method: "DELETE",
      headers: { Authorization: "Bearer tok_station" },
    })
    expect(clearStationDb).toHaveBeenCalledOnce()
    expect(clearCaches).toHaveBeenCalledOnce()
    expect(localStorage.getItem(DEVICE_TOKEN_KEY)).toBeNull()
    expect(localStorage.getItem(DEVICE_TYPE_KEY)).toBeNull()
    expect(redirect).toHaveBeenCalledWith("/station")
  })

  it("logs the POS out against its own route and clears the queue", async () => {
    localStorage.setItem(DEVICE_TOKEN_KEY, "tok_pos")
    localStorage.setItem(DEVICE_TYPE_KEY, "pos")
    sessionStorage.setItem(POS_QUEUE_KEY, JSON.stringify([{ status: "pending" }]))
    const fetchFn = vi.fn().mockResolvedValue({ ok: true, status: 204 } as Response)
    const redirect = vi.fn()

    await logoutDevice("pos", { fetchFn, redirect })

    expect(fetchFn).toHaveBeenCalledWith("/api/v1/pos/me", {
      method: "DELETE",
      headers: { Authorization: "Bearer tok_pos" },
    })
    expect(sessionStorage.getItem(POS_QUEUE_KEY)).toBeNull()
    expect(localStorage.getItem(DEVICE_TOKEN_KEY)).toBeNull()
    expect(redirect).toHaveBeenCalledWith("/pos")
  })

  it("still clears locally and redirects when the server revoke fails (offline)", async () => {
    localStorage.setItem(DEVICE_TOKEN_KEY, "tok_station")
    const fetchFn = vi.fn().mockRejectedValue(new Error("offline"))
    const clearStationDb = vi.fn().mockResolvedValue(undefined)
    const clearCaches = vi.fn().mockResolvedValue(undefined)
    const redirect = vi.fn()

    await logoutDevice("station", { fetchFn, clearStationDb, clearCaches, redirect })

    expect(clearStationDb).toHaveBeenCalledOnce()
    expect(localStorage.getItem(DEVICE_TOKEN_KEY)).toBeNull()
    expect(redirect).toHaveBeenCalledWith("/station")
  })

  it("skips the server call when there is no token but still wipes and redirects", async () => {
    const fetchFn = vi.fn()
    const clearStationDb = vi.fn().mockResolvedValue(undefined)
    const clearCaches = vi.fn().mockResolvedValue(undefined)
    const redirect = vi.fn()

    await logoutDevice("station", { fetchFn, token: null, clearStationDb, clearCaches, redirect })

    expect(fetchFn).not.toHaveBeenCalled()
    expect(clearStationDb).toHaveBeenCalledOnce()
    expect(redirect).toHaveBeenCalledWith("/station")
  })
})

describe("unsyncedWorkCount", () => {
  it("counts queued station redemptions from the outbox", async () => {
    const outbox = memoryOutbox()
    await outbox.enqueue({ orderId: "o1", productIds: ["p1"], tenantId: "t1" }, 1)
    await outbox.enqueue({ orderId: "o2", productIds: ["p2"], tenantId: "t1" }, 2)

    expect(await unsyncedWorkCount("station", outbox)).toBe(2)
  })

  it("counts only non-synced POS orders", async () => {
    sessionStorage.setItem(
      POS_QUEUE_KEY,
      JSON.stringify([{ status: "pending" }, { status: "failed" }, { status: "synced" }, { status: "paid" }])
    )

    expect(await unsyncedWorkCount("pos")).toBe(2)
  })

  it("returns 0 for an empty or unreadable POS queue", async () => {
    expect(await unsyncedWorkCount("pos")).toBe(0)
    sessionStorage.setItem(POS_QUEUE_KEY, "not json")
    expect(await unsyncedWorkCount("pos")).toBe(0)
  })
})
