import type { Cents } from "./common"

// Mirrors the backend refund/deposit engine (OpenAPI schemas/refunds.yaml +
// orders.yaml). Self-contained so the admin order detail and the refunds audit
// page share one source of truth.

export type AdminOrderStatus = "pending" | "paid" | "cancelled" | "refunded" | "partially_refunded"

export type AdminOrderItemType = "simple" | "bundle" | "component" | "deposit"

export interface OrderLineRedemption {
  id: string
  orderLineId: string
  redeemedAt: string
}

export interface AdminOrderLine {
  id: string
  orderId: string
  lineType: AdminOrderItemType
  productId: string
  title: string
  quantity: number
  unitPriceCents: Cents
  parentLineId?: string | null
  bundleSlotId?: string | null
  bundleSlotName?: string | null
  variantId?: string | null
  variantValue?: string | null
  productImage?: string | null
  productDescription?: string | null
  childLines?: AdminOrderLine[] | null
  redemption?: OrderLineRedemption | null
}

export interface OrderPaymentSummary {
  id?: string
  method?: string
  amountCents?: Cents
  paidAt?: string
  cardBrand?: string
  cardLast4?: string
  entryMode?: string
  cardTransactionId?: string
}

export interface AdminOrder {
  id: string
  customerId?: string | null
  contactEmail?: string | null
  totalCents: Cents
  status: AdminOrderStatus
  origin?: string | null
  createdAt: string
  updatedAt?: string
  qrPayload?: string | null
  paymentAttemptId?: string | null
  payrexxGatewayId?: number | null
  payrexxTransactionId?: number | null
  lines?: AdminOrderLine[] | null
  payments?: OrderPaymentSummary[] | null
}

export type RefundKind = "line" | "arbitrary"
export type RefundMethod = "cash" | "twint"
export type RefundActorType = "operator" | "admin"
export type RefundStatus = "pending" | "succeeded" | "failed"

export interface RefundLine {
  id: string
  orderLineId: string
  quantity: number
  amountCents: Cents
}

export interface Refund {
  id: string
  orderId: string
  amountCents: Cents
  kind: RefundKind
  method: RefundMethod
  reason?: string | null
  actorType: RefundActorType
  actorId: string
  deviceId?: string | null
  status: RefundStatus
  createdAt: string
  lines?: RefundLine[]
}

export interface RefundViewLine {
  orderLineId: string
  productId: string
  title: string
  variantId?: string | null
  variantValue?: string | null
  unitPriceCents: Cents
  // zero on a comped order, which collected nothing
  refundableUnitCents: Cents
  boughtQuantity: number
  refundedQuantity: number
  refundableQuantity: number
  redeemed: boolean
}

export interface RefundViewDeposit {
  orderLineId: string
  productId: string
  title: string
  unitPriceCents: Cents
  refundableUnitCents: Cents
  boughtQuantity: number
  returnedQuantity: number
  outstandingQuantity: number
}

export interface RefundView {
  order: AdminOrder
  lines: RefundViewLine[]
  deposits: RefundViewDeposit[]
  refundableBalanceCents: Cents
  refunds: Refund[]
}

export interface RefundList {
  items: Refund[]
}
