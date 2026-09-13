import type { BatchVoucherStatus } from "./api"

export const STATUS_LABEL: Record<BatchVoucherStatus, string> = {
  scheduled: "Geplant",
  active: "Aktiv",
  expired: "Abgelaufen",
  revoked: "Widerrufen",
  redeemed: "Eingelöst",
}

export const STATUS_CLASS: Record<BatchVoucherStatus, string> = {
  scheduled: "bg-amber-100 text-amber-900",
  active: "bg-emerald-100 text-emerald-900",
  expired: "bg-muted text-muted-foreground",
  revoked: "bg-destructive/10 text-destructive",
  redeemed: "bg-blue-100 text-blue-900",
}

export function StatusBadge({ status }: { status: BatchVoucherStatus }) {
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_CLASS[status]}`}>
      {STATUS_LABEL[status]}
    </span>
  )
}
