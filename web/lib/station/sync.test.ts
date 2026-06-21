import { describe, expect, it, vi } from "vitest"
import { flushOutbox } from "./sync"
import { memoryOutbox, type StationOutbox } from "./sync-queue"

type FetchCall = { url: string; init: RequestInit }

// Minimal fetch double: records calls and returns a configurable outcome.
// `ok` may be a boolean or a per-orderId map; `throws` simulates a network drop;
// `status` overrides the response status (defaults to 200 ok / 500 otherwise).
function fakeFetch(outcome: { ok?: boolean | Record<string, boolean>; throws?: boolean; status?: number }) {
  const calls: FetchCall[] = []
  const fn = vi.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init })
    if (outcome.throws) throw new Error("network down")
    const body = JSON.parse(String(init.body)) as { orderId: string }
    const ok = typeof outcome.ok === "object" ? (outcome.ok[body.orderId] ?? false) : (outcome.ok ?? true)
    return { ok, status: outcome.status ?? (ok ? 200 : 500) } as Response
  })
  return { fetch: fn as unknown as typeof fetch, calls }
}

async function seed(outbox: StationOutbox, orders: Array<{ orderId: string; productIds?: string[] }>, now = 1000) {
  for (const o of orders) {
    await outbox.enqueue({ orderId: o.orderId, tenantId: "tenant_seed1", productIds: o.productIds ?? ["pA"] }, now)
  }
}

describe("flushOutbox", () => {
  it("online direct-save: uploads with a stable Idempotency-Key and leaves no residue", async () => {
    const outbox = memoryOutbox()
    await seed(outbox, [{ orderId: "o1" }])
    const { fetch, calls } = fakeFetch({ ok: true })

    const summary = await flushOutbox({ outbox, fetch, token: () => "tok", now: () => 2000 })

    expect(summary).toMatchObject({ synced: 1, failed: 0, remaining: 0 })
    expect(await outbox.list()).toHaveLength(0)
    expect(calls).toHaveLength(1)
    expect(calls[0]!.url).toBe("/api/v1/stations/redeem")
    const headers = calls[0]!.init.headers as Record<string, string>
    expect(headers["Idempotency-Key"]).toBe("o1")
    expect(headers.Authorization).toBe("Bearer tok")
    expect(JSON.parse(String(calls[0]!.init.body))).toEqual({ orderId: "o1" })
  })

  it("a duplicate flush is a no-op: nothing left to send", async () => {
    const outbox = memoryOutbox()
    await seed(outbox, [{ orderId: "o1" }])
    const first = fakeFetch({ ok: true })
    await flushOutbox({ outbox, fetch: first.fetch, token: () => "tok", now: () => 2000 })

    const second = fakeFetch({ ok: true })
    const summary = await flushOutbox({ outbox, fetch: second.fetch, token: () => "tok", now: () => 3000 })

    expect(summary).toMatchObject({ synced: 0, remaining: 0 })
    expect(second.calls).toHaveLength(0)
  })

  it("Gherkin: syncs queued redemptions when connectivity returns, never dropping the entry", async () => {
    const outbox = memoryOutbox()
    await seed(outbox, [{ orderId: "o1" }], 1000)

    // Offline: the upload fails and the item stays queued with backoff.
    const down = fakeFetch({ throws: true })
    const failed = await flushOutbox({ outbox, fetch: down.fetch, token: () => "tok", now: () => 1000 })
    expect(failed).toMatchObject({ synced: 0, failed: 1, remaining: 1 })
    const item = (await outbox.list())[0]!
    expect(item.attempts).toBe(1)
    expect(item.nextAttemptAt).toBeGreaterThan(1000)

    // Still in backoff: not yet due, so no attempt is made (entry preserved).
    const early = fakeFetch({ ok: true })
    const skipped = await flushOutbox({ outbox, fetch: early.fetch, token: () => "tok", now: () => 2000 })
    expect(skipped).toMatchObject({ synced: 0, skipped: 1, remaining: 1 })
    expect(early.calls).toHaveLength(0)

    // Connectivity returns and the backoff has elapsed: it uploads and clears.
    const up = fakeFetch({ ok: true })
    const synced = await flushOutbox({ outbox, fetch: up.fetch, token: () => "tok", now: () => 99_999 })
    expect(synced).toMatchObject({ synced: 1, remaining: 0 })
    expect(await outbox.list()).toHaveLength(0)
  })

  it("partial-batch success: keeps the failures, removes the successes", async () => {
    const outbox = memoryOutbox()
    await seed(outbox, [{ orderId: "o1" }, { orderId: "o2" }, { orderId: "o3" }])
    const { fetch } = fakeFetch({ ok: { o1: true, o2: false, o3: true } })

    const summary = await flushOutbox({ outbox, fetch, token: () => "tok", now: () => 2000 })

    expect(summary).toMatchObject({ synced: 2, failed: 1, remaining: 1 })
    const remaining = await outbox.list()
    expect(remaining.map((i) => i.orderId)).toEqual(["o2"])
    expect(remaining[0]!.attempts).toBe(1)
  })

  it("skips the flush without dropping items when there is no usable token", async () => {
    const outbox = memoryOutbox()
    await seed(outbox, [{ orderId: "o1" }])
    const { fetch, calls } = fakeFetch({ ok: true })

    const summary = await flushOutbox({ outbox, fetch, token: () => null, now: () => 2000 })

    expect(summary).toMatchObject({ synced: 0, skipped: 1, remaining: 1 })
    expect(calls).toHaveLength(0)
    expect(await outbox.list()).toHaveLength(1)
  })

  it("auth rejection is terminal: stops the pass and leaves items queued without bumping backoff", async () => {
    const outbox = memoryOutbox()
    await seed(outbox, [{ orderId: "o1" }, { orderId: "o2" }])
    const { fetch, calls } = fakeFetch({ ok: false, status: 401 })

    const summary = await flushOutbox({ outbox, fetch, token: () => "stale", now: () => 2000 })

    // The first 401 short-circuits the rest of the pass — no further requests are
    // made with the rejected token (the fetch wrapper redirects to re-pair).
    expect(calls).toHaveLength(1)
    expect(summary).toMatchObject({ synced: 0, failed: 2, remaining: 2 })
    // Both items remain, immediately due (no backoff), to flush after re-pairing.
    const remaining = await outbox.list()
    expect(remaining.map((i) => i.orderId).sort()).toEqual(["o1", "o2"])
    expect(remaining.every((i) => i.attempts === 0)).toBe(true)
  })

  it("is re-entrant: overlapping triggers share one pass instead of double-sending", async () => {
    const outbox = memoryOutbox()
    await seed(outbox, [{ orderId: "o1" }])
    const { fetch, calls } = fakeFetch({ ok: true })

    const deps = { outbox, fetch, token: () => "tok", now: () => 2000 }
    const p1 = flushOutbox(deps)
    const p2 = flushOutbox(deps)
    expect(p1).toBe(p2)
    await Promise.all([p1, p2])

    expect(calls).toHaveLength(1)
  })
})
