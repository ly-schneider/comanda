import { describe, expect, it } from "vitest"
import type { Refund, RefundView } from "@/types/refund"
import { buildPickupSlip, buildRefundSlip, mapRefundMethod, planRefundReceipts } from "./refund-receipts"

function refund(overrides: Partial<Refund> = {}): Refund {
  return {
    id: "r1",
    orderId: "order00000001",
    amountCents: 400,
    kind: "line",
    method: "cash",
    actorType: "operator",
    actorId: "dev1",
    status: "succeeded",
    createdAt: "2026-06-30T10:00:00Z",
    ...overrides,
  }
}

function view(overrides: Partial<RefundView> = {}): RefundView {
  return {
    order: {
      id: "order00000001",
      totalCents: 800,
      status: "partially_refunded",
      createdAt: "2026-06-30T09:00:00Z",
      qrPayload: "newtoken",
    },
    lines: [
      {
        orderLineId: "l1",
        productId: "p1",
        title: "Kaffee",
        unitPriceCents: 200,
        boughtQuantity: 2,
        refundedQuantity: 1,
        refundableQuantity: 1,
        redeemed: false,
      },
    ],
    deposits: [],
    refundableBalanceCents: 400,
    refunds: [],
    ...overrides,
  }
}

describe("mapRefundMethod", () => {
  it("routes online TWINT to twint and everything else to cash-out", () => {
    expect(mapRefundMethod("twint")).toBe("twint")
    expect(mapRefundMethod("cash")).toBe("cash-out")
  })
})

describe("slip builders", () => {
  it("builds a refund slip with negative-rendered amount and mapped method", () => {
    const slip = buildRefundSlip({ refund: refund({ amountCents: 250, method: "twint" }) })
    expect(slip.kind).toBe("refund")
    expect(slip.method).toBe("twint")
    expect(slip.refundCents).toBe(250)
    expect(slip.orderId).toBe("order00000001")
  })

  it("builds a pickup slip carrying the new QR", () => {
    const slip = buildPickupSlip({ orderId: "o1", pickupQr: "tok", method: "cash-out", items: [] })
    expect(slip.kind).toBe("pickup")
    expect(slip.pickupQr).toBe("tok")
  })
})

describe("planRefundReceipts", () => {
  it("prints refund + new-QR pickup slip and collects the old receipt on a product refund", () => {
    const plan = planRefundReceipts({ action: "line", refund: refund(), view: view() })
    expect(plan.slips.map((s) => s.kind)).toEqual(["refund", "pickup"])
    expect(plan.collectOldReceipt).toBe(true)
    const pickup = plan.slips[1]!
    expect(pickup.pickupQr).toBe("newtoken")
    expect(pickup.items).toEqual([{ title: "Kaffee", quantity: 1, unitPriceCents: 200 }])
  })

  it("does not swap the receipt when the QR is unchanged (redeemed-line refund)", () => {
    const plan = planRefundReceipts({
      action: "line",
      refund: refund(),
      view: view({
        order: {
          id: "order00000001",
          totalCents: 800,
          status: "partially_refunded",
          createdAt: "x",
          qrPayload: "sametoken",
        },
      }),
      previousQr: "sametoken",
    })
    expect(plan.slips.map((s) => s.kind)).toEqual(["refund"])
    expect(plan.collectOldReceipt).toBe(false)
  })

  it("omits the pickup slip when no QR was re-issued", () => {
    const plan = planRefundReceipts({
      action: "line",
      refund: refund(),
      view: view({
        order: { id: "order00000001", totalCents: 800, status: "refunded", createdAt: "x", qrPayload: null },
      }),
    })
    expect(plan.slips.map((s) => s.kind)).toEqual(["refund"])
    expect(plan.collectOldReceipt).toBe(true)
  })

  it("prints only a refund slip and keeps the QR on a deposit return", () => {
    const plan = planRefundReceipts({ action: "deposit", refund: refund({ amountCents: 200 }), view: view() })
    expect(plan.slips.map((s) => s.kind)).toEqual(["refund"])
    expect(plan.collectOldReceipt).toBe(false)
  })

  it("prints a refund slip (no void) on cancel and collects the old receipt", () => {
    const plan = planRefundReceipts({ action: "cancel", refund: refund({ amountCents: 800 }), view: view() })
    expect(plan.slips.map((s) => s.kind)).toEqual(["refund"])
    expect(plan.collectOldReceipt).toBe(true)
  })
})
