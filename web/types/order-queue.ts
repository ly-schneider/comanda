import type { Cents } from "./common"

export type OrderSyncStatus = "pending" | "syncing" | "synced" | "failed" | "paid"

export type PosPaymentMethod =
  | "cash"
  | "card"
  | "twint"
  | "gratis_guest"
  | "gratis_vip"
  | "gratis_staff"
  | "gratis_plugin"

export interface GratisInfo {
  type: "guest" | "vip" | "staff"
}

export interface PluginPayment {
  key: string
  data: unknown
}

export interface QueuedOrderItem {
  productId: string
  quantity: number
  menuSelections?: Array<{ slotId: string; productId: string }>
  variantId?: string
}

export interface ReceiptItem {
  title: string
  quantity: number
  unitPriceCents: Cents
  configuration?: Array<{ slot: string; choice: string }>
}

export interface CardMeta {
  brand?: string
  last4?: string
  entryMode?: string
  transactionId?: string
}

export interface QueuedOrder {
  localId: string
  idempotencyKey: string
  items: QueuedOrderItem[]
  totalCents: Cents
  paymentMethod: PosPaymentMethod
  gratisInfo?: GratisInfo
  cardMeta?: CardMeta
  plugin?: PluginPayment
  status: OrderSyncStatus
  serverId?: string
  // Signed Ed25519 pickup token from order create; printed as the QR when present.
  serverQrPayload?: string
  attemptCount: number
  lastAttemptAt?: number
  lastError?: string
  receiptData?: {
    items: ReceiptItem[]
    pickupQr: string | null
    orderTimestamp: number
  }
  createdAt: number
}

export interface OrderQueueState {
  orders: QueuedOrder[]
  isOnline: boolean
}
