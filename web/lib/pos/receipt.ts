import type { PosPaymentMethod } from "@/types/order-queue"

// Shared contract for the ESC/POS slips rendered natively by the Android POS
// (T7). The web side only assembles this JSON and hands it to the bridge; the
// app decides layout per `kind`. Field names are part of that contract — keep
// them in sync with the Android receipt renderer.

export type ReceiptBranding = {
  displayName: string
  logoUrl: string | null
  address: string | null
  currency: string
}

// Slip variant for the native renderer; absent ⇒ "sale".
export type ReceiptKind = "sale" | "refund" | "pickup" | "void"

// Card and cash refunds are handed back in person (cash-out); only online TWINT
// refunds move money automatically.
export type ReceiptRefundMethod = "cash-out" | "twint"

export type Receipt = {
  kind?: ReceiptKind
  method: PosPaymentMethod | ReceiptRefundMethod
  totalCents: number
  // Deposit portion included in totalCents (sale slips).
  depositCents?: number
  // Refund magnitude for refund/void slips; rendered negative.
  refundCents?: number
  orderId?: string
  items?: Array<{
    title: string
    quantity: number
    unitPriceCents: number
    configuration?: Array<{ slot: string; choice: string }>
  }>
  pickupQr?: string | null
  branding?: ReceiptBranding
}

export type PosBridge = {
  print?: (s: string) => void
  payWithCard?: (p: { amountCents: number; currency: string; reference: string } | string) => void
}

export function getPosBridge(): PosBridge | undefined {
  return (globalThis as unknown as { PosBridge?: PosBridge }).PosBridge
}

// True when a native printer bridge is reachable (Android POS WebView).
export function canPrintReceipt(): boolean {
  return typeof getPosBridge()?.print === "function"
}

// Fire-and-forget print; the bridge reports success/failure via the
// `comanda:print:result` window event.
export function printReceipt(receipt: Receipt): void {
  try {
    getPosBridge()?.print?.(JSON.stringify(receipt))
  } catch {}
}
