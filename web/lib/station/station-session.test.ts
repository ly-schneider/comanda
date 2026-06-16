import { describe, expect, it, vi } from "vitest"
import { memoryKVStore } from "@/lib/station/offline-db"
import {
  cacheStationSession,
  getCachedStationSession,
  resolveStationSession,
  STATION_ME_URL,
  type StationStatus,
} from "@/lib/station/station-session"

const TOKEN = "opaque-device-token"

const APPROVED: StationStatus = {
  id: "st_1",
  status: "approved",
  name: "Pommes-Stand",
  products: [{ productId: "p1", name: "Pommes" }],
}

function res(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as unknown as Response
}

describe("resolveStationSession", () => {
  it("returns no-token and never touches the network without a token", async () => {
    const fetchFn = vi.fn()
    const result = await resolveStationSession({
      token: null,
      online: true,
      fetchFn: fetchFn as unknown as typeof fetch,
      store: memoryKVStore(),
      now: () => 1,
    })
    expect(result).toEqual({ kind: "no-token" })
    expect(fetchFn).not.toHaveBeenCalled()
  })

  it("resolves from the server and caches the identity when online", async () => {
    const store = memoryKVStore()
    const fetchFn = vi.fn(async () => res(200, APPROVED))
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: fetchFn as unknown as typeof fetch,
      store,
      now: () => 42,
    })
    expect(result).toEqual({ kind: "resolved", status: APPROVED, source: "network" })
    expect(fetchFn).toHaveBeenCalledWith(STATION_ME_URL, { headers: { Authorization: `Bearer ${TOKEN}` } })
    expect(await getCachedStationSession(store)).toEqual({ status: APPROVED, cachedAt: 42 })
  })

  it("reports system-disabled on a 503", async () => {
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: (async () => res(503, {})) as unknown as typeof fetch,
      store: memoryKVStore(),
      now: () => 1,
    })
    expect(result).toEqual({ kind: "system-disabled" })
  })

  it("treats a 401/403 as authoritative and clears the cached identity", async () => {
    const store = memoryKVStore()
    await cacheStationSession(APPROVED, 1, store)
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: (async () => res(403, {})) as unknown as typeof fetch,
      store,
      now: () => 1,
    })
    expect(result).toEqual({ kind: "unauthorized" })
    expect(await getCachedStationSession(store)).toBeNull()
  })

  it("falls back to the cached identity when the network fails mid-request", async () => {
    const store = memoryKVStore()
    await cacheStationSession(APPROVED, 5, store)
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: (async () => {
        throw new TypeError("Failed to fetch")
      }) as unknown as typeof fetch,
      store,
      now: () => 9,
    })
    expect(result).toEqual({ kind: "resolved", status: APPROVED, source: "cache" })
  })

  it("reports offline-unprepared when the network fails and nothing is cached", async () => {
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: (async () => {
        throw new Error("down")
      }) as unknown as typeof fetch,
      store: memoryKVStore(),
      now: () => 1,
    })
    expect(result).toEqual({ kind: "offline-unprepared" })
  })

  it("degrades to the cached identity on a 5xx without re-minting it as a network resolve", async () => {
    const store = memoryKVStore()
    await cacheStationSession(APPROVED, 5, store)
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: (async () => res(500, {})) as unknown as typeof fetch,
      store,
      now: () => 9,
    })
    expect(result).toEqual({ kind: "resolved", status: APPROVED, source: "cache" })
    // A 5xx must not clear the cached identity — only an explicit 401/403 does.
    expect(await getCachedStationSession(store)).toEqual({ status: APPROVED, cachedAt: 5 })
  })

  it("reports offline-unprepared on a 5xx when nothing is cached", async () => {
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: (async () => res(502, {})) as unknown as typeof fetch,
      store: memoryKVStore(),
      now: () => 1,
    })
    expect(result).toEqual({ kind: "offline-unprepared" })
  })

  it("falls back to cache when a 200 body is unparseable", async () => {
    const store = memoryKVStore()
    await cacheStationSession(APPROVED, 3, store)
    const result = await resolveStationSession({
      token: TOKEN,
      online: true,
      fetchFn: (async () => ({
        ok: true,
        status: 200,
        json: async () => {
          throw new SyntaxError("Unexpected token")
        },
      })) as unknown as typeof fetch,
      store,
      now: () => 9,
    })
    expect(result).toEqual({ kind: "resolved", status: APPROVED, source: "cache" })
  })

  it("resolves from cache offline and makes zero network calls", async () => {
    const store = memoryKVStore()
    await cacheStationSession(APPROVED, 7, store)
    const fetchFn = vi.fn()
    const result = await resolveStationSession({
      token: TOKEN,
      online: false,
      fetchFn: fetchFn as unknown as typeof fetch,
      store,
      now: () => 1,
    })
    expect(result).toEqual({ kind: "resolved", status: APPROVED, source: "cache" })
    expect(fetchFn).not.toHaveBeenCalled()
  })

  it("reports offline-unprepared offline with no cache, still without any network call", async () => {
    const fetchFn = vi.fn()
    const result = await resolveStationSession({
      token: TOKEN,
      online: false,
      fetchFn: fetchFn as unknown as typeof fetch,
      store: memoryKVStore(),
      now: () => 1,
    })
    expect(result).toEqual({ kind: "offline-unprepared" })
    expect(fetchFn).not.toHaveBeenCalled()
  })
})
