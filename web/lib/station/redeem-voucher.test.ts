import { beforeEach, describe, expect, it, vi } from "vitest"
import type { EntitlementPayload } from "./qr-verify"
import { redeemVoucherScan, type ServerRedeemResult, type VoucherRedeemDeps } from "./redeem-voucher"
import type { VoucherLedger, VoucherRedemption } from "./voucher-ledger"

const TENANT = "tenant_seed1"
const VOUCHER = "Vch7Ab2Cd9Ef"
const BURGER = "prod_burger1"
const DRINK = "prod_cola001"
const NOW = 1_700_050_000_000

const payload: EntitlementPayload = {
  tenantId: TENANT,
  voucherId: VOUCHER,
  validFrom: 1_700_000_000,
  validUntil: 1_700_086_400,
  label: "Outreach",
  lines: [{ productId: BURGER, quantity: 1 }],
}

function memoryLedger(): VoucherLedger {
  const rows = new Map<string, VoucherRedemption>()
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
      return Array.from(rows.values()).filter((r) => r.syncedAt == null && r.failedAt == null)
    },
    async markSynced(id, orderId, at) {
      const row = rows.get(id)
      if (row) rows.set(id, { ...row, orderId, syncedAt: at })
    },
    async markFailed(id, reason, at) {
      const existing = rows.get(id)
      if (existing) rows.set(id, { ...existing, failedAt: at, failedReason: reason })
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

let ledger: VoucherLedger
let server: ReturnType<typeof vi.fn>

beforeEach(() => {
  ledger = memoryLedger()
  server = vi.fn(async (): Promise<ServerRedeemResult> => ({
    ok: true,
    orderId: "ord_00000001",
    alreadyRedeemed: false,
  }))
})

function deps(overrides: Partial<VoucherRedeemDeps> = {}): VoucherRedeemDeps {
  return {
    verify: () => ({ ok: true, payload }),
    stationTenantId: TENANT,
    assignedProductIds: new Set([BURGER]),
    catalog: new Map([[BURGER, { name: "Burger" }]]),
    ledger,
    revoked: () => false,
    redeemOnServer: server as unknown as VoucherRedeemDeps["redeemOnServer"],
    now: () => NOW,
    ...overrides,
  }
}

describe("redeemVoucherScan", () => {
  it("redeems a valid slip, records it locally and mints the order server-side", async () => {
    const result = await redeemVoucherScan("tok", deps())

    expect(result).toEqual({
      kind: "redeemed",
      pending: false,
      lines: [{ productId: BURGER, title: "Burger", productImage: null, quantity: 1 }],
    })
    expect(server).toHaveBeenCalledWith(VOUCHER)
    expect(await ledger.get(VOUCHER)).toMatchObject({ orderId: "ord_00000001", syncedAt: NOW })
  })

  it("refuses a second scan of the same slip on this device", async () => {
    await redeemVoucherScan("tok", deps())
    server.mockClear()

    const second = await redeemVoucherScan("tok", deps())

    expect(second).toMatchObject({ kind: "already-redeemed", redeemedAt: NOW })
    expect(server).not.toHaveBeenCalled()
  })

  it("records the redemption as pending when the server is unreachable", async () => {
    server.mockResolvedValue({ ok: false, reason: "unreachable" })

    const result = await redeemVoucherScan("tok", deps())

    expect(result).toMatchObject({ kind: "redeemed", pending: true })
    expect(await ledger.pending()).toHaveLength(1)
  })

  it("releases the claim when the product is sold out, so the slip stays alive", async () => {
    server.mockResolvedValue({ ok: false, reason: "refused", soldOut: true, message: "Burger ausverkauft" })

    const result = await redeemVoucherScan("tok", deps())

    expect(result).toMatchObject({ kind: "sold-out", message: "Burger ausverkauft" })
    expect(await ledger.get(VOUCHER)).toBeNull()
  })

  it("re-redeems after a sold-out release once stock is back", async () => {
    server.mockResolvedValueOnce({ ok: false, reason: "refused", soldOut: true })
    await redeemVoucherScan("tok", deps())

    server.mockResolvedValueOnce({ ok: true, orderId: "ord_00000002", alreadyRedeemed: false })
    const retry = await redeemVoucherScan("tok", deps())

    expect(retry).toMatchObject({ kind: "redeemed", pending: false })
  })

  it("shows the server's reason verbatim rather than claiming the slip is blocked", async () => {
    server.mockResolvedValue({
      ok: false,
      reason: "refused",
      message: "Gutschein gültig — nicht mehr im Sortiment: Burger",
    })

    const result = await redeemVoucherScan("tok", deps())

    expect(result).toMatchObject({ kind: "refused", message: "Gutschein gültig — nicht mehr im Sortiment: Burger" })
    expect(await ledger.get(VOUCHER)).toBeNull()
  })

  it("falls back to revoked only when the server gives no reason", async () => {
    server.mockResolvedValue({ ok: false, reason: "refused" })

    expect(await redeemVoucherScan("tok", deps())).toEqual({ kind: "revoked" })
  })

  it("refuses a slip the server says was already spent at another stand", async () => {
    server.mockResolvedValue({ ok: true, orderId: "ord_00000001", alreadyRedeemed: true })

    const result = await redeemVoucherScan("tok", deps())

    expect(result).toMatchObject({ kind: "already-redeemed" })
    expect(result.kind).not.toBe("redeemed")
  })

  it("does not report a redemption when the device is not authenticated", async () => {
    server.mockResolvedValue({ ok: false, reason: "auth" })

    const result = await redeemVoucherScan("tok", deps())

    expect(result).toMatchObject({ kind: "auth-failed" })
    expect(await ledger.get(VOUCHER)).toBeNull()
  })

  it("rejects a slip signed for another tenant", async () => {
    const result = await redeemVoucherScan("tok", deps({ stationTenantId: "tenant_other" }))

    expect(result).toEqual({ kind: "wrong-event" })
    expect(server).not.toHaveBeenCalled()
  })

  it("rejects a revoked slip even inside its validity window", async () => {
    const result = await redeemVoucherScan("tok", deps({ revoked: (id) => id === VOUCHER }))

    expect(result).toEqual({ kind: "revoked" })
  })

  it("reports a slip that is not valid yet", async () => {
    const result = await redeemVoucherScan("tok", deps({ now: () => 1_600_000_000_000 }))

    expect(result).toEqual({ kind: "scheduled", validFrom: payload.validFrom })
  })

  it("reports an expired slip", async () => {
    const result = await redeemVoucherScan("tok", deps({ now: () => 1_800_000_000_000 }))

    expect(result).toEqual({ kind: "expired", validUntil: payload.validUntil })
  })

  it("refuses a stand that serves only part of the entitlement", async () => {
    const twoLines: EntitlementPayload = {
      ...payload,
      lines: [
        { productId: BURGER, quantity: 1 },
        { productId: DRINK, quantity: 1 },
      ],
    }

    const result = await redeemVoucherScan("tok", deps({ verify: () => ({ ok: true, payload: twoLines }) }))

    expect(result).toMatchObject({ kind: "not-for-stand" })
    expect(server).not.toHaveBeenCalled()
    expect(await ledger.get(VOUCHER)).toBeNull()
  })

  it("surfaces a verification failure without touching the ledger", async () => {
    const result = await redeemVoucherScan("tok", deps({ verify: () => ({ ok: false, reason: "bad-signature" }) }))

    expect(result).toEqual({ kind: "invalid", reason: "bad-signature" })
    expect(await ledger.list()).toHaveLength(0)
  })

  it("falls back to the raw product id when the catalog has no entry", async () => {
    const result = await redeemVoucherScan("tok", deps({ catalog: new Map() }))

    expect(result).toMatchObject({ kind: "redeemed", lines: [expect.objectContaining({ title: BURGER })] })
  })
})
