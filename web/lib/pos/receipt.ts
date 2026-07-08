import type { PosPaymentMethod } from "@/types/order-queue"

// Field names are part of the contract with the Android receipt renderer — keep in sync.
export type ReceiptBranding = {
  displayName: string
  logoUrl: string | null
  address: string | null
  currency: string
}

// Absent ⇒ "sale".
export type ReceiptKind = "sale" | "refund" | "pickup"

export type ReceiptRefundMethod = "cash-out" | "twint"

export type Receipt = {
  kind?: ReceiptKind
  method: PosPaymentMethod | ReceiptRefundMethod
  totalCents: number
  // Included in totalCents (sale slips).
  depositCents?: number
  // Rendered negative on refund slips.
  refundCents?: number
  orderId?: string
  cardTransactionId?: string
  items?: Array<{
    title: string
    quantity: number
    unitPriceCents: number
    configuration?: Array<{ slot: string; choice: string }>
  }>
  pickupQr?: string | null
  branding?: ReceiptBranding
}

export type CardCheckoutPayload = {
  amountCents: number
  currency: string
  reference: string
  title?: string
  items?: string
}

export type PosBridge = {
  print?: (s: string) => void
  payWithCard?: (p: CardCheckoutPayload | string) => void
}

export function buildCardCheckoutSummary(items: NonNullable<Receipt["items"]>): {
  title: string
  details: string
} {
  const title = items.map((it) => `${it.quantity}× ${it.title}`).join(", ")
  const details = items
    .map((it) => {
      const cfg = it.configuration?.map((c) => `${c.slot}: ${c.choice}`).join(", ")
      return cfg ? `${it.quantity}× ${it.title} (${cfg})` : `${it.quantity}× ${it.title}`
    })
    .join("\n")
  return { title, details }
}

export function getPosBridge(): PosBridge | undefined {
  return (globalThis as unknown as { PosBridge?: PosBridge }).PosBridge
}

export function canPrintReceipt(): boolean {
  return typeof getPosBridge()?.print === "function"
}

// Fire-and-forget; results arrive on the "comanda:print:result" window event.
export function printReceipt(receipt: Receipt): void {
  try {
    getPosBridge()?.print?.(JSON.stringify(receipt))
  } catch {}
}
