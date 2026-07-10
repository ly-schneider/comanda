import { describe, expect, it } from "vitest"
import type { RefundView } from "@/types/refund"
import { buildPickupSlip, planRefundReceipts } from "./refund-receipts"

function view(overrides: Partial<RefundView> = {}): RefundView {
  return {
    order: {
      id: "order00000001",
      totalCents: 800,
      status: "partially_refunded",
      createdAt: "2026-06-30T09:00:00Z",
      qrPayload: "newtoken",
      payments: [{ method: "card" }],
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

describe("buildPickupSlip", () => {
  it("builds a pickup slip carrying the new QR", () => {
    const slip = buildPickupSlip({ orderId: "o1", pickupQr: "tok", method: "cash", items: [] })
    expect(slip.kind).toBe("pickup")
    expect(slip.pickupQr).toBe("tok")
  })
})

describe("planRefundReceipts", () => {
  it("reprints only the re-issued pickup slip on a product refund and collects the old receipt", () => {
    const plan = planRefundReceipts({ action: "line", view: view() })
    expect(plan.slips.map((s) => s.kind)).toEqual(["pickup"])
    expect(plan.collectOldReceipt).toBe(true)
    const pickup = plan.slips[0]!
    expect(pickup.pickupQr).toBe("newtoken")
    expect(pickup.items).toEqual([{ title: "Kaffee", quantity: 1, unitPriceCents: 200 }])
  })

  it("carries the product variant into the reprinted pickup line title", () => {
    const plan = planRefundReceipts({
      action: "line",
      view: view({
        lines: [
          {
            orderLineId: "l1",
            productId: "p1",
            title: "Glace",
            variantValue: "Vanille",
            unitPriceCents: 200,
            boughtQuantity: 2,
            refundedQuantity: 1,
            refundableQuantity: 1,
            redeemed: false,
          },
        ],
      }),
    })
    expect(plan.slips[0]!.items).toEqual([{ title: "Glace (Vanille)", quantity: 1, unitPriceCents: 200 }])
  })

  it("mirrors the original sale's payment method on the pickup slip, not the refund's", () => {
    const plan = planRefundReceipts({
      action: "line",
      view: view({
        order: {
          id: "order00000001",
          totalCents: 800,
          status: "partially_refunded",
          createdAt: "x",
          qrPayload: "newtoken",
          payments: [{ method: "twint" }],
        },
      }),
    })
    expect(plan.slips[0]!.method).toBe("twint")
  })

  it("prints nothing when the QR is unchanged (redeemed-line refund)", () => {
    const plan = planRefundReceipts({
      action: "line",
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
    expect(plan.slips).toEqual([])
    expect(plan.collectOldReceipt).toBe(false)
  })

  it("prints nothing but collects the old receipt when the order is terminal", () => {
    const plan = planRefundReceipts({
      action: "line",
      view: view({
        order: { id: "order00000001", totalCents: 800, status: "refunded", createdAt: "x", qrPayload: null },
      }),
    })
    expect(plan.slips).toEqual([])
    expect(plan.collectOldReceipt).toBe(true)
  })

  it("prints nothing and keeps the QR on a deposit return", () => {
    const plan = planRefundReceipts({ action: "deposit", view: view() })
    expect(plan.slips).toEqual([])
    expect(plan.collectOldReceipt).toBe(false)
  })

  it("prints nothing on cancel and collects the old receipt", () => {
    const plan = planRefundReceipts({ action: "cancel", view: view() })
    expect(plan.slips).toEqual([])
    expect(plan.collectOldReceipt).toBe(true)
  })
})
