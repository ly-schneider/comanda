import type {
  CardMeta,
  CreateOrderResult,
  GratisInfo,
  OrderSyncStatus,
  PluginPayment,
  PosPaymentMethod,
  PreCreatedOrder,
  QueuedOrder,
  QueuedOrderItem,
  ReceiptItem,
} from "@/types/order-queue"
import { generateLocalId } from "./idempotency"

const STORAGE_KEY = "comanda-pos-order-queue"
const MAX_RETRY_ATTEMPTS = 5
const RETRY_DELAYS = [2000, 4000, 8000, 16000, 32000]
const SYNC_INTERVAL = 30000

const PERMANENT_ERROR_CODES = ["plugin_validation_failed"]

type SyncListener = (order: QueuedOrder) => void
type StateListener = (state: { orders: QueuedOrder[]; isOnline: boolean }) => void
type PaymentErrorListener = (order: QueuedOrder, errorCode: string, errorMessage: string) => void

export class OrderQueueManager {
  private orders: QueuedOrder[] = []
  private isOnline = true
  private syncTimeout: ReturnType<typeof setTimeout> | null = null
  private syncListeners: Set<SyncListener> = new Set()
  private stateListeners: Set<StateListener> = new Set()
  private paymentErrorListeners: Set<PaymentErrorListener> = new Set()
  private token: string
  private deviceId: string
  private isSyncing = false

  constructor(token: string, deviceId: string) {
    this.token = token
    this.deviceId = deviceId
    this.loadFromStorage()
    this.setupNetworkListeners()
    this.startPeriodicSync()
  }

  private loadFromStorage() {
    try {
      const stored = sessionStorage.getItem(STORAGE_KEY)
      if (stored) {
        const parsed = JSON.parse(stored) as QueuedOrder[]
        this.orders = parsed.filter((o) => o.status !== "paid")
      }
    } catch {
      this.orders = []
    }
  }

  private saveToStorage() {
    try {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify(this.orders))
    } catch {}
  }

  private setupNetworkListeners() {
    if (typeof window === "undefined") return

    const updateOnline = () => {
      this.isOnline = navigator.onLine
      this.notifyStateListeners()
      if (this.isOnline) this.triggerSync()
    }

    window.addEventListener("online", updateOnline)
    window.addEventListener("offline", updateOnline)
    this.isOnline = navigator.onLine
  }

  private startPeriodicSync() {
    if (typeof window === "undefined") return

    const scheduleNext = () => {
      this.syncTimeout = setTimeout(() => {
        this.triggerSync()
        scheduleNext()
      }, SYNC_INTERVAL)
    }
    scheduleNext()
  }

  private notifyStateListeners() {
    const state = { orders: [...this.orders], isOnline: this.isOnline }
    Array.from(this.stateListeners).forEach((listener) => {
      try {
        listener(state)
      } catch {}
    })
  }

  private notifySyncListeners(order: QueuedOrder) {
    Array.from(this.syncListeners).forEach((listener) => {
      try {
        listener(order)
      } catch {}
    })
  }

  private notifyPaymentErrorListeners(order: QueuedOrder, errorCode: string, errorMessage: string) {
    Array.from(this.paymentErrorListeners).forEach((listener) => {
      try {
        listener(order, errorCode, errorMessage)
      } catch {}
    })
  }

  // Creates the pending order online up front (before the tender modal) so the
  // cashier sees out-of-stock before taking money; enqueue later runs only the pay step.
  async createOrderNow(items: QueuedOrderItem[]): Promise<CreateOrderResult> {
    const idempotencyKey = generateLocalId()
    try {
      const res = await fetch(`/api/v1/orders`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${this.token}`,
          "Idempotency-Key": idempotencyKey,
        },
        body: JSON.stringify({ items }),
      })
      const json = (await res.json().catch(() => ({}))) as {
        id?: string
        qrPayload?: string | null
        code?: string
        message?: string
        detail?: string
        details?: { items?: Array<{ productId: string; variantId?: string; available?: number }> }
      }
      if (res.status === 409 && json.code === "out_of_stock") {
        return { ok: false, code: "out_of_stock", soldOut: json.details?.items ?? [] }
      }
      if (!res.ok || !json.id) {
        return { ok: false, code: "failed", message: json.message || json.detail }
      }
      return { ok: true, serverId: json.id, serverQrPayload: json.qrPayload ?? undefined, idempotencyKey }
    } catch {
      return { ok: false, code: "network" }
    }
  }

  async discardOrder(serverId: string): Promise<void> {
    try {
      await fetch(`/api/v1/orders/${serverId}/discard`, {
        method: "POST",
        headers: { Authorization: `Bearer ${this.token}` },
      })
    } catch {}
  }

  enqueue(
    items: QueuedOrderItem[],
    totalCents: number,
    paymentMethod: PosPaymentMethod,
    receiptData?: { items: ReceiptItem[]; pickupQr: string | null },
    gratisInfo?: GratisInfo,
    cardMeta?: CardMeta,
    plugin?: PluginPayment,
    preCreated?: PreCreatedOrder
  ): QueuedOrder {
    const now = Date.now()
    const localId = generateLocalId()

    const order: QueuedOrder = {
      localId,
      // Pre-created orders keep their original create key so the derived ":pay"
      // key stays stable across retries.
      idempotencyKey: preCreated?.idempotencyKey ?? localId,
      items,
      totalCents,
      paymentMethod,
      gratisInfo,
      cardMeta,
      plugin,
      serverId: preCreated?.serverId,
      serverQrPayload: preCreated?.serverQrPayload,
      // Always "pending" so triggerSync runs it; a present serverId makes
      // syncOrder skip the create step and only run the pay step.
      status: "pending",
      attemptCount: 0,
      createdAt: now,
      receiptData: receiptData
        ? {
            ...receiptData,
            orderTimestamp: now,
          }
        : undefined,
    }

    this.orders.push(order)
    this.saveToStorage()
    this.notifyStateListeners()

    // The create step already happened online, so the print/jeton coordination
    // (keyed off this event by syncOrder for non-pre-created orders) must still
    // fire. Defer to a microtask so the caller's pendingPrints entry is set first.
    if (preCreated && typeof window !== "undefined") {
      queueMicrotask(() => {
        window.dispatchEvent(new CustomEvent("comanda:pos:order-server-id", { detail: order }))
      })
    }

    this.triggerSync()

    return order
  }

  triggerSync() {
    if (this.isSyncing || !this.isOnline) return

    const pending = this.orders.filter(
      (o) => o.status === "pending" || (o.status === "failed" && o.attemptCount < MAX_RETRY_ATTEMPTS)
    )
    if (pending.length === 0) return

    this.isSyncing = true
    this.syncNextOrder(pending, 0)
  }

  private async syncNextOrder(pending: QueuedOrder[], index: number) {
    if (index >= pending.length) {
      this.isSyncing = false
      return
    }

    const order = pending[index]
    if (!order) {
      this.isSyncing = false
      return
    }

    const shouldRetry =
      order.status === "failed" &&
      order.lastAttemptAt &&
      Date.now() - order.lastAttemptAt < RETRY_DELAYS[Math.min(order.attemptCount - 1, RETRY_DELAYS.length - 1)]!

    if (shouldRetry) {
      this.syncNextOrder(pending, index + 1)
      return
    }

    await this.syncOrder(order)
    this.syncNextOrder(pending, index + 1)
  }

  private async syncOrder(order: QueuedOrder) {
    const idx = this.orders.findIndex((o) => o.localId === order.localId)
    if (idx === -1) return

    this.orders[idx] = { ...order, status: "syncing" as OrderSyncStatus }
    this.saveToStorage()
    this.notifyStateListeners()

    try {
      // Pre-created orders already have a serverId (created online up front); the
      // queue only runs their pay step. Others create the order here first.
      let serverId = this.orders[idx]!.serverId
      if (!serverId) {
        const resOrder = await fetch(`/api/v1/orders`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${this.token}`,
            "Idempotency-Key": order.idempotencyKey,
          },
          body: JSON.stringify({ items: order.items }),
        })

        const orderJson = (await resOrder.json()) as { id?: string; qrPayload?: string | null; detail?: string }
        if (!resOrder.ok || !orderJson.id) {
          throw new Error(orderJson.detail || "order_failed")
        }

        serverId = orderJson.id
        this.orders[idx] = {
          ...this.orders[idx]!,
          serverId,
          serverQrPayload: orderJson.qrPayload ?? undefined,
          status: "synced",
        }
        this.saveToStorage()
        this.notifyStateListeners()

        if (typeof window !== "undefined") {
          window.dispatchEvent(new CustomEvent("comanda:pos:order-server-id", { detail: this.orders[idx]! }))
        }
      }

      const paymentKey = `${order.idempotencyKey}:pay`
      const paymentBody: Record<string, unknown> = { method: order.paymentMethod, channel: "pos" }
      if (order.plugin) {
        paymentBody.plugin = order.plugin
      }
      if (order.paymentMethod === "card" && order.cardMeta) {
        paymentBody.card = order.cardMeta
      }
      const resPay = await fetch(`/api/v1/orders/${serverId}/payment`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${this.token}`,
          "Idempotency-Key": paymentKey,
        },
        body: JSON.stringify(paymentBody),
      })

      const payJson = (await resPay.json()) as { code?: string; message?: string; detail?: string }
      if (!resPay.ok) {
        const errorCode = payJson.code || "payment_failed"
        const errorMessage = payJson.message || payJson.detail || "payment_failed"

        if (PERMANENT_ERROR_CODES.includes(errorCode)) {
          this.orders[idx] = {
            ...this.orders[idx]!,
            status: "failed",
            attemptCount: MAX_RETRY_ATTEMPTS,
            lastAttemptAt: Date.now(),
            lastError: errorCode,
          }
          this.saveToStorage()
          this.notifyStateListeners()
          this.notifyPaymentErrorListeners(this.orders[idx]!, errorCode, errorMessage)
          return
        }

        throw new Error(errorMessage)
      }

      this.orders[idx] = { ...this.orders[idx]!, status: "paid" }
      this.saveToStorage()
      this.notifyStateListeners()
      this.notifySyncListeners(this.orders[idx]!)
    } catch (e) {
      const currentOrder = this.orders[idx]!
      this.orders[idx] = {
        ...currentOrder,
        status: "failed",
        attemptCount: currentOrder.attemptCount + 1,
        lastAttemptAt: Date.now(),
        lastError: e instanceof Error ? e.message : "sync_failed",
      }
      this.saveToStorage()
      this.notifyStateListeners()

      if (this.orders[idx]!.attemptCount < MAX_RETRY_ATTEMPTS) {
        const delay = RETRY_DELAYS[Math.min(this.orders[idx]!.attemptCount - 1, RETRY_DELAYS.length - 1)]!
        setTimeout(() => this.triggerSync(), delay)
      }
    }
  }

  getOrder(localId: string): QueuedOrder | undefined {
    return this.orders.find((o) => o.localId === localId)
  }

  getPendingOrders(): QueuedOrder[] {
    return this.orders.filter((o) => o.status === "pending" || o.status === "syncing")
  }

  getFailedOrders(): QueuedOrder[] {
    return this.orders.filter((o) => o.status === "failed" && o.attemptCount >= MAX_RETRY_ATTEMPTS)
  }

  retryOrder(localId: string) {
    const idx = this.orders.findIndex((o) => o.localId === localId)
    if (idx === -1) return

    this.orders[idx] = { ...this.orders[idx]!, status: "pending", attemptCount: 0, lastError: undefined }
    this.saveToStorage()
    this.notifyStateListeners()
    this.triggerSync()
  }

  deleteOrder(localId: string) {
    this.orders = this.orders.filter((o) => o.localId !== localId)
    this.saveToStorage()
    this.notifyStateListeners()
  }

  clearPaidOrders() {
    this.orders = this.orders.filter((o) => o.status !== "paid")
    this.saveToStorage()
    this.notifyStateListeners()
  }

  onSync(listener: SyncListener): () => void {
    this.syncListeners.add(listener)
    return () => this.syncListeners.delete(listener)
  }

  onPaymentError(listener: PaymentErrorListener): () => void {
    this.paymentErrorListeners.add(listener)
    return () => this.paymentErrorListeners.delete(listener)
  }

  onStateChange(listener: StateListener): () => void {
    this.stateListeners.add(listener)
    listener({ orders: [...this.orders], isOnline: this.isOnline })
    return () => this.stateListeners.delete(listener)
  }

  changePaymentMethod(localId: string, newMethod: PosPaymentMethod, gratisInfo?: GratisInfo) {
    const idx = this.orders.findIndex((o) => o.localId === localId)
    if (idx === -1) return

    this.orders[idx] = {
      ...this.orders[idx]!,
      paymentMethod: newMethod,
      gratisInfo: gratisInfo,
      status: "pending",
      attemptCount: 0,
      lastError: undefined,
      idempotencyKey: `${this.orders[idx]!.idempotencyKey}:retry${Date.now()}`,
    }
    this.saveToStorage()
    this.notifyStateListeners()
    this.triggerSync()
  }

  getState() {
    return { orders: [...this.orders], isOnline: this.isOnline }
  }

  destroy() {
    if (this.syncTimeout) {
      clearTimeout(this.syncTimeout)
    }
    this.syncListeners.clear()
    this.stateListeners.clear()
    this.paymentErrorListeners.clear()
  }
}
