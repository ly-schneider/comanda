import { beforeEach, describe, expect, it, vi } from "vitest"
import type { VoucherLedger, VoucherRedemption } from "./voucher-ledger"
import { flushVoucherRedemptions } from "./voucher-sync"

const TENANT = "tenant_seed1"
const NOW = 1_700_050_000_000

function row(voucherId: string, overrides: Partial<VoucherRedemption> = {}): VoucherRedemption {
  return {
    voucherId,
    tenantId: TENANT,
    label: "Outreach",
    lines: [{ productId: "prod_burger1", quantity: 1 }],
    redeemedAt: NOW - 60_000,
    attempts: 0,
    ...overrides,
  }
}

function memoryLedger(seed: VoucherRedemption[] = []): VoucherLedger {
  const rows = new Map(seed.map((r) => [r.voucherId, r]))
  return {
    async get(id) {
      return rows.get(id) ?? null
    },
    async claim(entry) {
      if (rows.has(entry.voucherId)) return "duplicate"
      rows.set(entry.voucherId, entry)
      return "claimed"
    },
    async pending() {
      return Array.from(rows.values()).filter((r) => r.syncedAt == null)
    },
    async markSynced(id, orderId, at) {
      const existing = rows.get(id)
      if (existing) rows.set(id, { ...existing, orderId, syncedAt: at })
    },
    async remove(id) {
      rows.delete(id)
    },
    async put(entry) {
      rows.set(entry.voucherId, entry)
    },
    async list() {
      return Array.from(rows.values())
    },
    async clear() {
      rows.clear()
    },
  }
}

function res(status: number, body: unknown = {}): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn()
})

function deps(ledger: VoucherLedger) {
  return {
    ledger,
    fetch: fetchMock as unknown as typeof fetch,
    token: () => "device-token",
    now: () => NOW,
  }
}

describe("flushVoucherRedemptions", () => {
  it("marks a redemption synced with the order the server minted", async () => {
    const ledger = memoryLedger([row("Vch000000001")])
    fetchMock.mockResolvedValue(res(200, { orderId: "ord_00000001" }))

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ synced: 1, failed: 0, released: 0 })
    expect(await ledger.get("Vch000000001")).toMatchObject({ orderId: "ord_00000001", syncedAt: NOW })
    expect(await ledger.list()).toHaveLength(1)
  })

  it("sends the voucher id as the idempotency key", async () => {
    fetchMock.mockResolvedValue(res(200, { orderId: "ord_00000001" }))

    await flushVoucherRedemptions(deps(memoryLedger([row("Vch000000001")])))

    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect((init.headers as Record<string, string>)["Idempotency-Key"]).toBe("Vch000000001")
  })

  it("keeps a row pending and backs off on a server error", async () => {
    const ledger = memoryLedger([row("Vch000000001")])
    fetchMock.mockResolvedValue(res(503))

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ synced: 0, failed: 1 })
    const stored = await ledger.get("Vch000000001")
    expect(stored).toMatchObject({ attempts: 1, lastAttemptAt: NOW })
    expect(stored?.nextAttemptAt).toBeGreaterThan(NOW)
    expect(await ledger.pending()).toHaveLength(1)
  })

  it("keeps a row pending when the network throws", async () => {
    const ledger = memoryLedger([row("Vch000000001")])
    fetchMock.mockRejectedValue(new Error("offline"))

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ failed: 1 })
    expect(await ledger.pending()).toHaveLength(1)
  })

  it("releases the claim when the server terminally refuses it", async () => {
    const ledger = memoryLedger([row("Vch000000001")])
    fetchMock.mockResolvedValue(res(409, { code: "voucher_product_sold_out" }))

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ released: 1, synced: 0 })
    expect(await ledger.get("Vch000000001")).toBeNull()
  })

  it("treats a rate limit as transient, not terminal", async () => {
    const ledger = memoryLedger([row("Vch000000001")])
    fetchMock.mockResolvedValue(res(429))

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ released: 0, failed: 1 })
    expect(await ledger.get("Vch000000001")).not.toBeNull()
  })

  it("stops after an auth rejection instead of burning the rest", async () => {
    const ledger = memoryLedger([row("Vch000000001"), row("Vch000000002")])
    fetchMock.mockResolvedValue(res(401))

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ synced: 0, failed: 2 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(await ledger.pending()).toHaveLength(2)
  })

  it("skips rows still inside their backoff window", async () => {
    const ledger = memoryLedger([row("Vch000000001", { attempts: 2, nextAttemptAt: NOW + 30_000 })])

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ skipped: 1, synced: 0 })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("leaves everything queued when there is no device token", async () => {
    const ledger = memoryLedger([row("Vch000000001")])

    const summary = await flushVoucherRedemptions({ ...deps(ledger), token: () => null })

    expect(summary).toMatchObject({ skipped: 1, remaining: 1 })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("keeps the row pending when a 2xx carries no order id", async () => {
    const ledger = memoryLedger([row("Vch000000001")])
    fetchMock.mockResolvedValue(res(200, {}))

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ synced: 0, failed: 1 })
    expect(await ledger.pending()).toHaveLength(1)
  })

  it("never syncs an already-synced row twice", async () => {
    const ledger = memoryLedger([row("Vch000000001", { syncedAt: NOW - 1000, orderId: "ord_00000001" })])

    const summary = await flushVoucherRedemptions(deps(ledger))

    expect(summary).toMatchObject({ synced: 0, remaining: 0 })
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
