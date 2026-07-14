import { describe, expect, it, vi } from "vitest"
import { memoryKVStore } from "./offline-db"
import {
  EMPTY_REVOCATIONS,
  loadRevocations,
  reconcileRevocations,
  REVOCATIONS_KEY,
  revokedIdSet,
  type ServerRevocations,
  syncedLabel,
} from "./revocations"

function res(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response
}

describe("loadRevocations / revokedIdSet", () => {
  it("defaults to empty and derives the id set from the cached feed", async () => {
    const store = memoryKVStore()
    expect(await loadRevocations(store)).toEqual(EMPTY_REVOCATIONS)

    await store.set<ServerRevocations>(REVOCATIONS_KEY, { ids: ["srv_aaaaaaa1", "srv_bbbbbbb1"], syncedAt: 1000 })

    const state = await loadRevocations(store)
    expect(revokedIdSet(state)).toEqual(new Set(["srv_aaaaaaa1", "srv_bbbbbbb1"]))
  })
})

describe("syncedLabel", () => {
  it("labels a never-synced feed and formats the sync age", () => {
    expect(syncedLabel(null, 5000)).toBe("Noch nicht synchronisiert")
    expect(syncedLabel(4000, 5000)).toBe("Zuletzt synchronisiert vor 1 Sek.")
    expect(syncedLabel(0, 120_000)).toBe("Zuletzt synchronisiert vor 2 Min.")
  })
})

describe("reconcileRevocations", () => {
  it("REPLACES the cached set on a successful pull", async () => {
    const store = memoryKVStore()
    await store.set<ServerRevocations>(REVOCATIONS_KEY, { ids: ["stale_id_001"], syncedAt: 1 })

    const fetchFn = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) =>
      res(200, { revokedIds: ["srv_new_0001", "srv_new_0002"], asOf: "2026-07-11T00:00:00Z" })
    )
    const out = await reconcileRevocations({
      fetchFn: fetchFn as unknown as typeof fetch,
      token: () => "dev",
      now: () => 5000,
      store,
    })

    expect(out).toEqual({ ids: ["srv_new_0001", "srv_new_0002"], syncedAt: 5000 })
    expect(await store.get(REVOCATIONS_KEY)).toEqual({ ids: ["srv_new_0001", "srv_new_0002"], syncedAt: 5000 })
    const headers = fetchFn.mock.calls[0]![1]!.headers as Record<string, string>
    expect(headers.Authorization).toBe("Bearer dev")
  })

  it("is a no-op without a token", async () => {
    const store = memoryKVStore()
    const fetchFn = vi.fn()

    const out = await reconcileRevocations({
      fetchFn: fetchFn as unknown as typeof fetch,
      token: () => null,
      now: () => 5000,
      store,
    })
    expect(out).toBeNull()
    expect(fetchFn).not.toHaveBeenCalled()
  })

  it("preserves the cache on a failed request (never clears without server truth)", async () => {
    const store = memoryKVStore()
    await store.set<ServerRevocations>(REVOCATIONS_KEY, { ids: ["keep_me_001"], syncedAt: 1 })

    for (const fetchFn of [
      vi.fn(async () => res(403, {})),
      vi.fn(async () => {
        throw new Error("offline")
      }),
    ]) {
      const out = await reconcileRevocations({
        fetchFn: fetchFn as unknown as typeof fetch,
        token: () => "dev",
        now: () => 5000,
        store,
      })
      expect(out).toBeNull()
      expect(await store.get(REVOCATIONS_KEY)).toEqual({ ids: ["keep_me_001"], syncedAt: 1 })
    }
  })
})
