import type { Receipt, ReceiptBranding, ReceiptRefundMethod } from "@/lib/pos/receipt"
import type { Refund, RefundMethod, RefundView } from "@/types/refund"

// Builds the ESC/POS slips printed after a POS refund / deposit return /
// cancel, matching the native renderer's per-`kind` contract (T7):
//   refund → items + "-amount" + method + order
//   pickup → remaining items + new QR + "Alte Quittung ungültig"
//   void   → "-amount" + method + order
// Card and cash refunds are cash-out; only original online TWINT routes back
// automatically. The backend decides; we only relabel for the slip.

export type ReceiptLineItem = NonNullable<Receipt["items"]>[number]

export type RefundAction = "line" | "deposit" | "cancel"

export interface RefundReceiptPlan {
  slips: Receipt[]
  // Whether the operator must physically take in the customer's old printed
  // receipt (its QR is superseded or voided). Deposit returns leave the pickup
  // QR untouched, so no collection.
  collectOldReceipt: boolean
}

export function mapRefundMethod(method: RefundMethod): ReceiptRefundMethod {
  return method === "twint" ? "twint" : "cash-out"
}

export function buildRefundSlip(args: {
  refund: Refund
  items?: ReceiptLineItem[]
  branding?: ReceiptBranding | null
}): Receipt {
  return {
    kind: "refund",
    method: mapRefundMethod(args.refund.method),
    totalCents: args.refund.amountCents,
    refundCents: args.refund.amountCents,
    orderId: args.refund.orderId,
    items: args.items,
    branding: args.branding ?? undefined,
  }
}

export function buildVoidSlip(args: { refund: Refund; branding?: ReceiptBranding | null }): Receipt {
  return {
    kind: "void",
    method: mapRefundMethod(args.refund.method),
    totalCents: args.refund.amountCents,
    refundCents: args.refund.amountCents,
    orderId: args.refund.orderId,
    branding: args.branding ?? undefined,
  }
}

export function buildPickupSlip(args: {
  orderId: string
  pickupQr: string
  method: ReceiptRefundMethod
  items?: ReceiptLineItem[]
  branding?: ReceiptBranding | null
}): Receipt {
  return {
    kind: "pickup",
    method: args.method,
    totalCents: 0,
    orderId: args.orderId,
    pickupQr: args.pickupQr,
    items: args.items,
    branding: args.branding ?? undefined,
  }
}

// Remaining still-collectable products, for the re-issued pickup slip.
function remainingItems(view: RefundView): ReceiptLineItem[] {
  return view.lines
    .filter((l) => l.refundableQuantity > 0)
    .map((l) => ({ title: l.title, quantity: l.refundableQuantity, unitPriceCents: l.unitPriceCents }))
}

export function planRefundReceipts(args: {
  action: RefundAction
  refund: Refund
  // The order view AFTER the action (carries the re-issued/voided QR + balances).
  view: RefundView
  // The order's pickup QR BEFORE the action, to detect a re-issue.
  previousQr?: string | null
  // Items just refunded, shown on the refund slip (optional).
  refundedItems?: ReceiptLineItem[]
  branding?: ReceiptBranding | null
}): RefundReceiptPlan {
  const { action, refund, view, previousQr, refundedItems, branding } = args

  if (action === "cancel") {
    return { slips: [buildVoidSlip({ refund, branding })], collectOldReceipt: true }
  }

  if (action === "deposit") {
    // Deposit return moves money but the pickup QR is unchanged.
    return { slips: [buildRefundSlip({ refund, items: refundedItems, branding })], collectOldReceipt: false }
  }

  // Product line refund: refund slip, plus a fresh pickup slip only when the
  // backend actually re-issued the QR (un-redeemed items). Refunding an
  // already-redeemed line leaves the QR untouched — no new slip, nothing to
  // physically swap.
  const slips: Receipt[] = [buildRefundSlip({ refund, items: refundedItems, branding })]
  const newQr = view.order.qrPayload
  const reissued = !!newQr && newQr !== (previousQr ?? null)
  const terminal = view.order.status === "cancelled" || view.order.status === "refunded"
  const items = remainingItems(view)
  if (reissued && items.length > 0 && !terminal) {
    slips.push(
      buildPickupSlip({
        orderId: view.order.id,
        pickupQr: newQr,
        method: mapRefundMethod(refund.method),
        items,
        branding,
      })
    )
  }
  // Collect the old receipt whenever its QR is superseded (re-issue) or the
  // order is now fully closed (the printed QR no longer entitles a pickup).
  return { slips, collectOldReceipt: reissued || terminal }
}
