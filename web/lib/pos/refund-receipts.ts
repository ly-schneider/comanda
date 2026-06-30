import type { Receipt, ReceiptBranding, ReceiptRefundMethod } from "@/lib/pos/receipt"
import type { Refund, RefundMethod, RefundView } from "@/types/refund"

export type ReceiptLineItem = NonNullable<Receipt["items"]>[number]

export type RefundAction = "line" | "deposit" | "cancel"

export interface RefundReceiptPlan {
  slips: Receipt[]
  // The old printed receipt must be physically taken in (QR superseded or order closed).
  collectOldReceipt: boolean
}

// card + cash → cash-out (handed back in person); online TWINT routes back automatically.
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

function remainingItems(view: RefundView): ReceiptLineItem[] {
  return view.lines
    .filter((l) => l.refundableQuantity > 0)
    .map((l) => ({ title: l.title, quantity: l.refundableQuantity, unitPriceCents: l.unitPriceCents }))
}

export function planRefundReceipts(args: {
  action: RefundAction
  refund: Refund
  // AFTER the action — carries the re-issued/voided QR.
  view: RefundView
  // the QR BEFORE the action, to detect a re-issue.
  previousQr?: string | null
  refundedItems?: ReceiptLineItem[]
  branding?: ReceiptBranding | null
}): RefundReceiptPlan {
  const { action, refund, view, previousQr, refundedItems, branding } = args

  if (action === "cancel") {
    return { slips: [buildVoidSlip({ refund, branding })], collectOldReceipt: true }
  }

  if (action === "deposit") {
    // A deposit return leaves the pickup QR unchanged.
    return { slips: [buildRefundSlip({ refund, items: refundedItems, branding })], collectOldReceipt: false }
  }

  // A new pickup slip only when the backend re-signed the QR (un-redeemed
  // refund); refunding an already-redeemed line leaves the QR untouched.
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
  return { slips, collectOldReceipt: reissued || terminal }
}
