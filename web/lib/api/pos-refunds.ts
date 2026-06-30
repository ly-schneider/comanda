import { deviceFetch } from "@/hooks/use-device-fetch"
import { readErrorMessage } from "@/lib/http"
import type { AdminOrder, Refund, RefundView } from "@/types/refund"

// Device-token (Bearer) auth carries no cookies, so these are not CSRF-protected.
// Arbitrary refunds are admin-only — no POS route.
export interface OrderLookupResult {
  items: AdminOrder[]
}

export interface RefundLineQty {
  orderLineId: string
  quantity: number
}

async function readJson<T>(res: Response): Promise<T> {
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return (await res.json()) as T
}

function posPath(orderId: string, suffix = ""): string {
  return `/api/v1/pos/orders/${encodeURIComponent(orderId)}${suffix}`
}

const jsonInit = (body: unknown): RequestInit => ({
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
})

// `q` is an order-id prefix or a signed pickup token; the backend verifies a token and returns its order.
export async function lookupPosOrders(q: string): Promise<OrderLookupResult> {
  const res = await deviceFetch(`/api/v1/pos/orders/lookup?q=${encodeURIComponent(q)}`)
  return readJson<OrderLookupResult>(res)
}

export async function getPosRefundView(orderId: string): Promise<RefundView> {
  const res = await deviceFetch(posPath(orderId, "/refunds"))
  return readJson<RefundView>(res)
}

export async function createPosLineRefund(orderId: string, lines: RefundLineQty[], reason?: string): Promise<Refund> {
  const res = await deviceFetch(posPath(orderId, "/refunds"), jsonInit({ lines, ...(reason ? { reason } : {}) }))
  return readJson<Refund>(res)
}

export async function createPosDepositReturn(
  orderId: string,
  lines: RefundLineQty[],
  token?: string | null,
  reason?: string
): Promise<Refund> {
  const res = await deviceFetch(
    posPath(orderId, "/deposit-returns"),
    jsonInit({ lines, ...(token ? { token } : {}), ...(reason ? { reason } : {}) })
  )
  return readJson<Refund>(res)
}

export async function cancelPosOrder(orderId: string, reason?: string): Promise<Refund> {
  const res = await deviceFetch(posPath(orderId, "/cancel"), jsonInit(reason ? { reason } : {}))
  return readJson<Refund>(res)
}
