import { apiRequest } from "../api"

export type OrderStatus = "pending" | "paid" | "cancelled" | "refunded" | "partially_refunded"

export interface OrderSummaryDTO {
  id: string
  status: OrderStatus
  createdAt: string
}

export interface ListResponse<T> {
  items: T[]
  count: number
}

export async function listMyOrders(accessToken?: string) {
  return apiRequest<ListResponse<OrderSummaryDTO>>("/v1/orders?scope=mine", {
    method: "GET",
    headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined,
  })
}

export interface OrderLineRedemptionDTO {
  id: string
  orderLineId: string
  redeemedAt: string
}

export interface OrderLineDTO {
  id: string
  orderId: string
  lineType: "simple" | "bundle" | "component" | "deposit"
  productId: string
  title: string
  quantity: number
  unitPriceCents: number
  /** Units of this line refunded/returned so far (deposits: units returned). Present only when > 0. */
  refundedQuantity?: number
  parentLineId?: string | null
  menuSlotId?: string | null
  menuSlotName?: string | null
  productImage?: string | null
  productDescription?: string | null
  childLines?: OrderLineDTO[] | null
  redemption?: OrderLineRedemptionDTO | null
}

export interface OrderPaymentSummaryDTO {
  id?: string
  method?: "CASH" | "CARD" | "TWINT"
  amountCents?: number
  paidAt?: string
  cardBrand?: string
  cardLast4?: string
  entryMode?: string
  cardTransactionId?: string
}

export interface PublicOrderDetailsDTO {
  id: string
  status: OrderStatus
  totalCents: number
  createdAt: string
  lines?: OrderLineDTO[]
  payments?: OrderPaymentSummaryDTO[]
  /** Total refunded so far in cents (succeeded refunds, incl. returned deposits). Present only when > 0. */
  refundedTotalCents?: number
  /** Signed, self-contained Ed25519 pickup token (offline-verifiable at the stand). */
  qrPayload?: string | null
}

export async function getOrderPublicById(orderId: string) {
  return apiRequest<PublicOrderDetailsDTO>(`/v1/orders/${encodeURIComponent(orderId)}`, {
    method: "GET",
  })
}
