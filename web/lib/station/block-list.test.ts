import { describe, expect, it, vi } from "vitest"
import {
  addManualBlock,
  effectiveBlockSet,
  isValidBlockId,
  loadBlockState,
  MANUAL_BLOCKS_KEY,
  reconcileRevocations,
  removeManualBlock,
  REVOCATIONS_KEY,
  type ServerRevocations,
} from "./block-list"
import { memoryKVStore } from "./offline-db"

function res(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response
}

describe("isValidBlockId", () => {
  it("accepts a 12-char mixed-case nanoid and rejects malformed ids", () => {
    expect(isValidBlockId("Vch7Ab2Cd9Ef")).toBe(true)
    expect(isValidBlockId("tenant_seed1")).toBe(true)
    expect(isValidBlockId("short")).toBe(false)
    expect(isValidBlockId("has spaces12")).toBe(false)
    // Confusable chars are outside the nanoid alphabet (0/O/o, I/i, l, j).
    expect(isValidBlockId("OOOOOOOOOOOO")).toBe(false)
    expect(isValidBlockId("")).toBe(false)
  })
})

describe("manual block list", () => {
  it("adds and removes ids verbatim (case-sensitive) and dedupes", async () => {
    const store = memoryKVStore()

    expect(await addManualBlock("Vch7Ab2Cd9Ef", store)).toEqual(["Vch7Ab2Cd9Ef"])
    // A different case is a different id — never coalesced.
    expect(await addManualBlock("vch7ab2cd9ef", store)).toEqual(["Vch7Ab2Cd9Ef", "vch7ab2cd9ef"])
    expect(await addManualBlock("Vch7Ab2Cd9Ef", store)).toEqual(["Vch7Ab2Cd9Ef", "vch7ab2cd9ef"])

    expect(await removeManualBlock("Vch7Ab2Cd9Ef", store)).toEqual(["vch7ab2cd9ef"])
    expect(await removeManualBlock("not_present1", store)).toEqual(["vch7ab2cd9ef"])
  })
})

describe("loadBlockState / effectiveBlockSet", () => {
  it("defaults to empty and unions server + manual", async () => {
    const store = memoryKVStore()
    expect(await loadBlockState(store)).toEqual({ server: { ids: [], syncedAt: null }, manual: [] })

    await store.set<ServerRevocations>(REVOCATIONS_KEY, { ids: ["srv_aaaaaaa1"], syncedAt: 1000 })
    await store.set<string[]>(MANUAL_BLOCKS_KEY, ["man_bbbbbbb1"])

    const state = await loadBlockState(store)
    expect(effectiveBlockSet(state)).toEqual(new Set(["srv_aaaaaaa1", "man_bbbbbbb1"]))
  })
})

describe("reconcileRevocations", () => {
  it("REPLACES the server set and CLEARS the manual list on a successful pull", async () => {
    const store = memoryKVStore()
    await store.set<ServerRevocations>(REVOCATIONS_KEY, { ids: ["stale_id_001"], syncedAt: 1 })
    await store.set<string[]>(MANUAL_BLOCKS_KEY, ["man_local_01"])

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
    expect(await store.get(MANUAL_BLOCKS_KEY)).toEqual([])
    const headers = fetchFn.mock.calls[0]![1]!.headers as Record<string, string>
    expect(headers.Authorization).toBe("Bearer dev")
  })

  it("is a no-op without a token", async () => {
    const store = memoryKVStore()
    await store.set<string[]>(MANUAL_BLOCKS_KEY, ["man_local_01"])
    const fetchFn = vi.fn()

    const out = await reconcileRevocations({
      fetchFn: fetchFn as unknown as typeof fetch,
      token: () => null,
      now: () => 5000,
      store,
    })
    expect(out).toBeNull()
    expect(fetchFn).not.toHaveBeenCalled()
    expect(await store.get(MANUAL_BLOCKS_KEY)).toEqual(["man_local_01"])
  })

  it("preserves the cache and manual list on a failed request (never clears without server truth)", async () => {
    const store = memoryKVStore()
    await store.set<ServerRevocations>(REVOCATIONS_KEY, { ids: ["keep_me_001"], syncedAt: 1 })
    await store.set<string[]>(MANUAL_BLOCKS_KEY, ["man_local_01"])

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
      expect(await store.get(MANUAL_BLOCKS_KEY)).toEqual(["man_local_01"])
    }
  })
})
