import type { PosPaymentMethod } from "@/types/order-queue"

// Field names are part of the contract with the Android receipt renderer — keep in sync.
export type ReceiptBranding = {
  displayName: string
  logoUrl: string | null
  address: string | null
  currency: string
}

// Absent ⇒ "sale".
export type ReceiptKind = "sale" | "refund" | "pickup" | "void"

export type ReceiptRefundMethod = "cash-out" | "twint"

export type Receipt = {
  kind?: ReceiptKind
  method: PosPaymentMethod | ReceiptRefundMethod
  totalCents: number
  // Included in totalCents (sale slips).
  depositCents?: number
  // Rendered negative on refund/void slips.
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

export function canPrintReceipt(): boolean {
  return typeof getPosBridge()?.print === "function"
}

// Fire-and-forget; results arrive on the "comanda:print:result" window event.
export function printReceipt(receipt: Receipt): void {
  try {
    getPosBridge()?.print?.(JSON.stringify(receipt))
  } catch {}
}
