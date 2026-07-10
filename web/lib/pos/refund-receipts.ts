import type { Receipt, ReceiptBranding } from "@/lib/pos/receipt"
import type { RefundView } from "@/types/refund"

export type ReceiptLineItem = NonNullable<Receipt["items"]>[number]

export type RefundAction = "line" | "deposit" | "cancel"

export interface RefundReceiptPlan {
  slips: Receipt[]
  // The old printed receipt must be physically taken in (QR superseded or order closed).
  collectOldReceipt: boolean
}

export function buildPickupSlip(args: {
  orderId: string
  pickupQr: string
  method: Receipt["method"] | ""
  items?: ReceiptLineItem[]
  branding?: ReceiptBranding | null
}): Receipt {
  return {
    kind: "pickup",
    method: args.method as Receipt["method"],
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
    .map((l) => ({
      title: l.variantValue ? `${l.title} (${l.variantValue})` : l.title,
      quantity: l.refundableQuantity,
      unitPriceCents: l.unitPriceCents,
    }))
}

// Refunds themselves are never printed; the only slip we produce is a fresh
// pickup ticket when a line refund left products to collect under a re-issued QR.
export function planRefundReceipts(args: {
  action: RefundAction
  // AFTER the action — carries the re-issued/voided QR.
  view: RefundView
  // the QR BEFORE the action, to detect a re-issue.
  previousQr?: string | null
  branding?: ReceiptBranding | null
}): RefundReceiptPlan {
  const { action, view, previousQr, branding } = args

  if (action === "cancel") {
    // Full cancellation: the order is terminal and its QR is void — collect the old receipt.
    return { slips: [], collectOldReceipt: true }
  }

  if (action === "deposit") {
    // A deposit return leaves the pickup QR unchanged; nothing to reprint.
    return { slips: [], collectOldReceipt: false }
  }

  // Reprint the pickup ticket only when the backend re-signed the QR (un-redeemed
  // refund); refunding an already-redeemed line leaves the QR untouched.
  const newQr = view.order.qrPayload
  const reissued = !!newQr && newQr !== (previousQr ?? null)
  const terminal = view.order.status === "cancelled" || view.order.status === "refunded"
  const items = remainingItems(view)
  const slips: Receipt[] = []
  if (reissued && items.length > 0 && !terminal) {
    slips.push(
      buildPickupSlip({
        orderId: view.order.id,
        pickupQr: newQr,
        // Pickup tickets mirror the original sale's payment method, not the refund's.
        method: (view.order.payments?.[0]?.method ?? "") as Receipt["method"] | "",
        items,
        branding,
      })
    )
  }
  return { slips, collectOldReceipt: reissued || terminal }
}
