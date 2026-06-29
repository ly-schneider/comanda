import type { AdminOrderStatus, RefundActorType, RefundKind, RefundMethod, RefundStatus } from "@/types/refund"

export const ORDER_STATUS_LABELS: Record<AdminOrderStatus, string> = {
  pending: "Ausstehend",
  paid: "Bezahlt",
  cancelled: "Storniert",
  refunded: "Erstattet",
  partially_refunded: "Teilweise erstattet",
}

export const ORDER_STATUS_COLORS: Record<AdminOrderStatus, string> = {
  pending: "bg-yellow-100 text-yellow-800",
  paid: "bg-green-100 text-green-800",
  cancelled: "bg-red-100 text-red-800",
  refunded: "bg-blue-100 text-blue-800",
  partially_refunded: "bg-blue-100 text-blue-800",
}

export const REFUND_KIND_LABELS: Record<RefundKind, string> = {
  line: "Rückerstattung",
  arbitrary: "Kulanz",
}

export const REFUND_METHOD_LABELS: Record<RefundMethod, string> = {
  cash: "Bargeld",
  twint: "TWINT",
}

export const REFUND_STATUS_LABELS: Record<RefundStatus, string> = {
  pending: "Ausstehend",
  succeeded: "Erfolgreich",
  failed: "Fehlgeschlagen",
}

export const REFUND_ACTOR_LABELS: Record<RefundActorType, string> = {
  operator: "Kasse",
  admin: "Admin",
}

export const REFUND_STATUS_COLORS: Record<RefundStatus, string> = {
  pending: "bg-yellow-100 text-yellow-800",
  succeeded: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
}

// Parses a CHF amount string ("12.50", "12,50") into cents, or null if invalid.
export function parseChfToCents(input: string): number | null {
  const norm = input.replace(",", ".").trim()
  if (!norm) return null
  const val = Number(norm)
  if (!isFinite(val) || Number.isNaN(val)) return null
  return Math.round(val * 100)
}
