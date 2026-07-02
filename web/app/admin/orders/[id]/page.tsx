"use client"
import { Minus, Plus } from "lucide-react"
import Image from "next/image"
import Link from "next/link"
import { useParams } from "next/navigation"
import { useEffect, useMemo, useState } from "react"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Textarea } from "@/components/ui/textarea"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { useMemberTier } from "@/hooks/use-member-tier"
import { getCSRFToken } from "@/lib/csrf"
import { readErrorMessage } from "@/lib/http"
import {
  ORDER_STATUS_COLORS,
  ORDER_STATUS_LABELS,
  parseChfToCents,
  REFUND_KIND_LABELS,
  REFUND_METHOD_LABELS,
  REFUND_STATUS_COLORS,
  REFUND_STATUS_LABELS,
} from "@/lib/refund-format"
import { formatChf } from "@/lib/utils"
import type {
  AdminOrderLine,
  AdminOrderStatus,
  OrderLineRedemption,
  Refund,
  RefundView,
  RefundViewDeposit,
  RefundViewLine,
} from "@/types/refund"

const REFUNDABLE_STATUSES = new Set<AdminOrderStatus>(["paid", "partially_refunded"])

// Pending orders keep the lightweight status flip (no money moves). Paid orders
// move money through the refund engine instead.
const PENDING_TRANSITIONS: AdminOrderStatus[] = ["paid", "cancelled"]

async function postJSON(fetchAuth: ReturnType<typeof useAuthorizedFetch>, url: string, body?: unknown): Promise<void> {
  const csrf = getCSRFToken()
  const res = await fetchAuth(url, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
    body: JSON.stringify(body ?? {}),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
}

export default function AdminOrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const fetchAuth = useAuthorizedFetch()
  const { isAppAdmin } = useMemberTier()
  const [view, setView] = useState<RefundView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  async function loadView() {
    setLoading(true)
    setError(null)
    try {
      const res = await fetchAuth(`/api/v1/orders/${encodeURIComponent(id)}/refunds`)
      if (!res.ok) throw new Error(await readErrorMessage(res))
      setView((await res.json()) as RefundView)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Fehler beim Laden der Bestellung")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    let cancelled = false
    void loadView().then(() => {
      if (cancelled) setView(null)
    })
    return () => {
      cancelled = true
    }
  }, [fetchAuth, id])

  const order = view?.order ?? null
  const orderItems = useMemo(() => order?.lines ?? [], [order])

  const grouped = useMemo(() => {
    if (!orderItems.length) return [] as Array<{ parent: AdminOrderLine; children: AdminOrderLine[] }>
    const childrenByParent: Record<string, AdminOrderLine[]> = {}
    for (const it of orderItems) {
      if (it.parentLineId) {
        ;(childrenByParent[it.parentLineId] ??= []).push(it)
      }
    }
    return orderItems
      .filter((it) => !it.parentLineId)
      .map((root) => ({ parent: root, children: root.childLines ?? childrenByParent[root.id] ?? [] }))
  }, [orderItems])

  const created = order?.createdAt ? new Date(order.createdAt).toLocaleString("de-CH") : "–"
  const updated = order?.updatedAt ? new Date(order.updatedAt).toLocaleString("de-CH") : "–"
  const origin = order?.origin || "shop"
  const primaryPayment = order?.payments?.[0]
  const posMethod = primaryPayment?.method
  const canRefund = order ? REFUNDABLE_STATUSES.has(order.status) : false

  return (
    <div className="min-w-0 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Bestellung</h1>
        <Link href="/admin/orders">
          <Button variant="outline">Zurück zur Übersicht</Button>
        </Link>
      </div>

      {loading && <div className="text-muted-foreground text-sm">Lade Bestellung…</div>}
      {error && <div className="text-sm text-red-600">{error}</div>}

      {order && (
        <div className="grid gap-4 md:grid-cols-2">
          <div className="rounded-md border p-4">
            <h2 className="mb-3 text-base font-semibold">Details</h2>
            <div className="space-y-1 text-sm">
              <div>
                <span className="text-muted-foreground">ID:</span> <span className="text-xs">{order.id}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">Status:</span>
                <span
                  className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${ORDER_STATUS_COLORS[order.status] || "bg-gray-100 text-gray-800"}`}
                >
                  {ORDER_STATUS_LABELS[order.status] || order.status}
                </span>
              </div>
              {order.status === "pending" && (
                <PendingStatusActions orderId={order.id} onDone={loadView} onError={setError} />
              )}
              <div>
                <span className="text-muted-foreground">Ursprung:</span> <span className="uppercase">{origin}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Summe:</span>{" "}
                <span className="font-medium">{formatChf(order.totalCents)}</span>
              </div>
              <div>
                <span className="text-muted-foreground">E-Mail:</span> {order.contactEmail || "–"}
              </div>
              <div>
                <span className="text-muted-foreground">Benutzer:</span>{" "}
                {order.customerId ? (
                  <Link
                    href={`/admin/users/${encodeURIComponent(order.customerId)}`}
                    className="text-xs underline underline-offset-2"
                  >
                    {order.customerId}
                  </Link>
                ) : (
                  "–"
                )}
              </div>
              <div>
                <span className="text-muted-foreground">Erstellt:</span> {created}
              </div>
              <div>
                <span className="text-muted-foreground">Aktualisiert:</span> {updated}
              </div>
            </div>
          </div>

          <div className="rounded-md border p-4">
            <h2 className="mb-3 text-base font-semibold">Zahlung</h2>
            <div className="space-y-1 text-sm">
              <div>
                <span className="text-muted-foreground">Methode:</span>{" "}
                <span className="uppercase">{posMethod || "–"}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Trans.-Nr.:</span>{" "}
                <span className="font-mono text-xs">{primaryPayment?.cardTransactionId || "–"}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Payrexx Gateway:</span>{" "}
                <span className="text-xs">{order.payrexxGatewayId ?? "–"}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Payrexx Txn:</span>{" "}
                <span className="text-xs">{order.payrexxTransactionId ?? "–"}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Payment Attempt:</span>{" "}
                <span className="text-xs">{order.paymentAttemptId || "–"}</span>
              </div>
            </div>
          </div>
        </div>
      )}

      {order && (
        <div>
          <h2 className="mb-3 text-lg font-semibold">Bestellte Artikel</h2>
          {grouped.length === 0 ? (
            <p className="text-muted-foreground text-sm">Keine Artikel gefunden.</p>
          ) : (
            <div className="flex flex-col gap-3">
              {grouped.map(({ parent, children }) => {
                const unitPrice = parent.unitPriceCents ?? 0
                const isBundle = parent.lineType === "bundle"
                return (
                  <div key={parent.id} className="rounded-xl border p-3">
                    <div className="flex items-center gap-3">
                      {parent.productImage && (
                        <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-[11px] bg-[#cec9c6]">
                          <Image
                            src={parent.productImage}
                            alt={"Produktbild von " + parent.title}
                            fill
                            sizes="64px"
                            quality={90}
                            unoptimized={parent.productImage.includes("localhost:10000")}
                            className="h-full w-full rounded-[11px] object-cover"
                          />
                        </div>
                      )}
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center justify-between gap-3">
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <p className="truncate font-medium">{parent.title}</p>
                              {!isBundle && <RedemptionBadge redemption={parent.redemption} />}
                            </div>
                          </div>
                          <div className="shrink-0 text-right">
                            <p className="text-muted-foreground text-sm">x{parent.quantity}</p>
                            <p className="font-medium">{formatChf(unitPrice * parent.quantity)}</p>
                          </div>
                        </div>
                      </div>
                    </div>
                    {children.length > 0 && (
                      <div className="mt-2 space-y-1">
                        {children.map((c) => (
                          <div key={c.id} className="flex items-center justify-between gap-2 pl-6 text-sm">
                            <div className="flex min-w-0 items-center gap-2">
                              <span className="truncate">
                                {c.menuSlotName ? `${c.menuSlotName}: ` : ""}
                                {c.title}
                              </span>
                              {c.lineType !== "deposit" && <RedemptionBadge redemption={c.redemption} />}
                            </div>
                            <span className="text-muted-foreground shrink-0 text-xs">
                              {c.lineType === "deposit" ? formatChf(c.unitPriceCents * c.quantity) : `x${c.quantity}`}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
          <div className="mt-4 flex items-center justify-between">
            <span className="text-muted-foreground text-sm">Summe</span>
            <span className="text-base font-semibold">{formatChf(order.totalCents)}</span>
          </div>
        </div>
      )}

      {view && canRefund && (
        <div className="space-y-6 rounded-md border p-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold">Rückerstattung</h2>
            <span className="text-muted-foreground text-sm">
              Erstattbar: <span className="text-foreground font-medium">{formatChf(view.refundableBalanceCents)}</span>
            </span>
          </div>

          <LineRefundSection
            orderId={order!.id}
            lines={view.lines}
            deposits={view.deposits}
            orderLines={orderItems}
            onDone={loadView}
          />

          <DepositReturnSection orderId={order!.id} deposits={view.deposits} onDone={loadView} />

          {isAppAdmin && (
            <ArbitraryRefundSection orderId={order!.id} maxCents={view.refundableBalanceCents} onDone={loadView} />
          )}

          <Separator />

          <CancelOrderButton orderId={order!.id} onDone={loadView} />
        </div>
      )}

      {view && view.refunds.length > 0 && <RefundHistory refunds={view.refunds} />}
    </div>
  )
}

function RedemptionBadge({ redemption }: { redemption?: OrderLineRedemption | null }) {
  if (!redemption) {
    return <span className="inline-flex rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600">Offen</span>
  }
  const at = new Date(redemption.redeemedAt).toLocaleString("de-CH")
  return (
    <span className="inline-flex rounded-full bg-green-100 px-2 py-0.5 text-xs text-green-700" title={at}>
      Eingelöst
    </span>
  )
}

function QtyStepper({
  value,
  max,
  disabled,
  onChange,
}: {
  value: number
  max: number
  disabled?: boolean
  onChange: (v: number) => void
}) {
  return (
    <div className="flex items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="h-8 w-8"
        disabled={disabled || value <= 0}
        onClick={() => onChange(Math.max(0, value - 1))}
        aria-label="Weniger"
      >
        <Minus className="h-4 w-4" />
      </Button>
      <span className="w-6 text-center text-sm tabular-nums">{value}</span>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="h-8 w-8"
        disabled={disabled || value >= max}
        onClick={() => onChange(Math.min(max, value + 1))}
        aria-label="Mehr"
      >
        <Plus className="h-4 w-4" />
      </Button>
    </div>
  )
}

function LineRefundSection({
  orderId,
  lines,
  deposits,
  orderLines,
  onDone,
}: {
  orderId: string
  lines: RefundViewLine[]
  deposits: RefundViewDeposit[]
  orderLines: AdminOrderLine[]
  onDone: () => Promise<void>
}) {
  const fetchAuth = useAuthorizedFetch()
  const [qty, setQty] = useState<Record<string, number>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refundable = lines.filter((l) => l.refundableQuantity > 0)

  // Each product line auto-refunds its linked deposit (capped at outstanding).
  const depositByParent = useMemo(() => {
    const m: Record<string, AdminOrderLine> = {}
    for (const l of orderLines) if (l.lineType === "deposit" && l.parentLineId) m[l.parentLineId] = l
    return m
  }, [orderLines])
  const outstandingByDeposit = useMemo(() => {
    const m: Record<string, number> = {}
    for (const d of deposits) m[d.orderLineId] = d.outstandingQuantity
    return m
  }, [deposits])

  const selected = refundable.map((l) => ({ line: l, q: qty[l.orderLineId] ?? 0 })).filter((x) => x.q > 0)
  const productCents = selected.reduce((s, x) => s + x.line.unitPriceCents * x.q, 0)
  const depositCents = selected.reduce((s, x) => {
    const dep = depositByParent[x.line.orderLineId]
    if (!dep) return s
    const outstanding = outstandingByDeposit[dep.id] ?? 0
    return s + Math.min(x.q, outstanding) * dep.unitPriceCents
  }, 0)

  if (refundable.length === 0) return null

  async function submit() {
    setBusy(true)
    setError(null)
    try {
      await postJSON(fetchAuth, `/api/v1/orders/${encodeURIComponent(orderId)}/refunds`, {
        lines: selected.map((x) => ({ orderLineId: x.line.orderLineId, quantity: x.q })),
      })
      setQty({})
      await onDone()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Rückerstattung fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold">Positionen erstatten</h3>
      <div className="flex flex-col gap-2">
        {refundable.map((l) => {
          const dep = depositByParent[l.orderLineId]
          return (
            <div key={l.orderLineId} className="flex items-center justify-between gap-3 rounded-md border p-2">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium">{l.title}</span>
                  {l.redeemed && (
                    <span className="inline-flex rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-800">
                      Bereits eingelöst
                    </span>
                  )}
                </div>
                <p className="text-muted-foreground text-xs">
                  {formatChf(l.unitPriceCents)} · noch {l.refundableQuantity} von {l.boughtQuantity}
                  {l.refundedQuantity > 0 ? ` · ${l.refundedQuantity} erstattet` : ""}
                  {dep ? ` · inkl. Depot ${formatChf(dep.unitPriceCents)}` : ""}
                </p>
              </div>
              <QtyStepper
                value={qty[l.orderLineId] ?? 0}
                max={l.refundableQuantity}
                disabled={busy}
                onChange={(v) => setQty((prev) => ({ ...prev, [l.orderLineId]: v }))}
              />
            </div>
          )
        })}
      </div>
      {error && <p className="text-destructive text-xs">{error}</p>}
      <div className="flex items-center justify-between">
        <span className="text-sm">
          {depositCents > 0 ? (
            <>
              Betrag: <span className="font-medium">{formatChf(productCents + depositCents)}</span>{" "}
              <span className="text-muted-foreground">(inkl. Depot {formatChf(depositCents)})</span>
            </>
          ) : (
            <>
              Betrag: <span className="font-medium">{formatChf(productCents)}</span>
            </>
          )}
        </span>
        <Button size="sm" disabled={busy || selected.length === 0} onClick={() => void submit()}>
          {busy ? "Wird erstattet…" : "Ausgewählte erstatten"}
        </Button>
      </div>
    </div>
  )
}

function DepositReturnSection({
  orderId,
  deposits,
  onDone,
}: {
  orderId: string
  deposits: RefundViewDeposit[]
  onDone: () => Promise<void>
}) {
  const fetchAuth = useAuthorizedFetch()
  const [qty, setQty] = useState<Record<string, number>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const returnable = deposits.filter((d) => d.outstandingQuantity > 0)
  const selected = returnable.map((d) => ({ deposit: d, q: qty[d.orderLineId] ?? 0 })).filter((x) => x.q > 0)
  const totalCents = selected.reduce((s, x) => s + x.deposit.unitPriceCents * x.q, 0)

  if (returnable.length === 0) return null

  async function submit() {
    setBusy(true)
    setError(null)
    try {
      await postJSON(fetchAuth, `/api/v1/orders/${encodeURIComponent(orderId)}/deposit-returns`, {
        lines: selected.map((x) => ({ orderLineId: x.deposit.orderLineId, quantity: x.q })),
      })
      setQty({})
      await onDone()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Depot-Rücknahme fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold">Depot zurücknehmen</h3>
      <div className="flex flex-col gap-2">
        {returnable.map((d) => (
          <div key={d.orderLineId} className="flex items-center justify-between gap-3 rounded-md border p-2">
            <div className="min-w-0">
              <span className="truncate text-sm font-medium">{d.title}</span>
              <p className="text-muted-foreground text-xs">
                {formatChf(d.unitPriceCents)} · noch {d.outstandingQuantity} von {d.boughtQuantity}
                {d.returnedQuantity > 0 ? ` · ${d.returnedQuantity} zurückgenommen` : ""}
              </p>
            </div>
            <QtyStepper
              value={qty[d.orderLineId] ?? 0}
              max={d.outstandingQuantity}
              disabled={busy}
              onChange={(v) => setQty((prev) => ({ ...prev, [d.orderLineId]: v }))}
            />
          </div>
        ))}
      </div>
      {error && <p className="text-destructive text-xs">{error}</p>}
      <div className="flex items-center justify-between">
        <span className="text-sm">
          Betrag: <span className="font-medium">{formatChf(totalCents)}</span>
        </span>
        <Button size="sm" variant="secondary" disabled={busy || selected.length === 0} onClick={() => void submit()}>
          {busy ? "Wird zurückgenommen…" : "Depot zurücknehmen"}
        </Button>
      </div>
    </div>
  )
}

function ArbitraryRefundSection({
  orderId,
  maxCents,
  onDone,
}: {
  orderId: string
  maxCents: number
  onDone: () => Promise<void>
}) {
  const fetchAuth = useAuthorizedFetch()
  const [amountInput, setAmountInput] = useState("")
  const [reason, setReason] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  if (maxCents <= 0) return null

  const amountCents = parseChfToCents(amountInput)
  const amountValid = amountCents != null && amountCents >= 1 && amountCents <= maxCents
  const valid = amountValid && reason.trim().length > 0

  async function submit() {
    if (!valid || amountCents == null) return
    setBusy(true)
    setError(null)
    try {
      await postJSON(fetchAuth, `/api/v1/orders/${encodeURIComponent(orderId)}/refunds/arbitrary`, {
        amountCents,
        reason: reason.trim(),
      })
      setAmountInput("")
      setReason("")
      await onDone()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Kulanz-Rückerstattung fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold">Kulanz-Rückerstattung</h3>
      <p className="text-muted-foreground text-xs">
        Freier Betrag mit Begründung, unabhängig von den Positionen. Bargeld wird vor Ort ausgezahlt.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor="arb-amount">Betrag (CHF)</Label>
          <Input
            id="arb-amount"
            inputMode="decimal"
            value={amountInput}
            placeholder="0.00"
            onChange={(e) => setAmountInput(e.target.value)}
            aria-invalid={amountInput.trim() !== "" && !amountValid}
          />
          <p className="text-muted-foreground text-xs">max. {formatChf(maxCents)}</p>
        </div>
        <div className="space-y-1">
          <Label htmlFor="arb-reason">Begründung</Label>
          <Textarea
            id="arb-reason"
            value={reason}
            rows={2}
            maxLength={500}
            placeholder="Pflichtfeld: Grund für die Kulanz"
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
      </div>
      {error && <p className="text-destructive text-xs">{error}</p>}
      <div className="flex justify-end">
        <Button size="sm" variant="secondary" disabled={busy || !valid} onClick={() => void submit()}>
          {busy ? "Wird erstattet…" : "Kulanz erstatten"}
        </Button>
      </div>
    </div>
  )
}

function CancelOrderButton({ orderId, onDone }: { orderId: string; onDone: () => Promise<void> }) {
  const fetchAuth = useAuthorizedFetch()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    setBusy(true)
    setError(null)
    try {
      await postJSON(fetchAuth, `/api/v1/orders/${encodeURIComponent(orderId)}/cancel`)
      setOpen(false)
      await onDone()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Stornierung fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">Bestellung stornieren</h3>
          <p className="text-muted-foreground text-xs">
            Erstattet den gesamten Restbetrag, stellt den Bestand wieder her und entwertet den Abhol-QR.
          </p>
        </div>
        <Button size="sm" variant="destructive" disabled={busy} onClick={() => setOpen(true)}>
          Stornieren
        </Button>
      </div>
      {error && <p className="text-destructive text-xs">{error}</p>}
      <AlertDialog open={open} onOpenChange={(o) => !busy && setOpen(o)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Bestellung stornieren?</AlertDialogTitle>
            <AlertDialogDescription>
              Der gesamte Restbetrag wird erstattet und der Abhol-QR entwertet. Diese Aktion kann nicht rückgängig
              gemacht werden.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              disabled={busy}
              onClick={(e) => {
                e.preventDefault()
                void submit()
              }}
            >
              {busy ? "Wird storniert…" : "Stornieren"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function RefundHistory({ refunds }: { refunds: Refund[] }) {
  return (
    <div className="space-y-3">
      <h2 className="text-lg font-semibold">Rückerstattungen</h2>
      <div className="flex flex-col gap-2">
        {refunds.map((r) => (
          <div key={r.id} className="flex items-start justify-between gap-3 rounded-md border p-3 text-sm">
            <div className="min-w-0 space-y-0.5">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{REFUND_KIND_LABELS[r.kind] ?? r.kind}</span>
                <span className="text-muted-foreground text-xs">{REFUND_METHOD_LABELS[r.method] ?? r.method}</span>
                <span
                  className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${REFUND_STATUS_COLORS[r.status] || "bg-gray-100 text-gray-800"}`}
                >
                  {REFUND_STATUS_LABELS[r.status] ?? r.status}
                </span>
              </div>
              <p className="text-muted-foreground text-xs">{new Date(r.createdAt).toLocaleString("de-CH")}</p>
              {r.reason && <p className="text-muted-foreground text-xs italic">«{r.reason}»</p>}
            </div>
            <span className="shrink-0 font-medium">{formatChf(r.amountCents)}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function PendingStatusActions({
  orderId,
  onDone,
  onError,
}: {
  orderId: string
  onDone: () => Promise<void>
  onError: (msg: string) => void
}) {
  const fetchAuth = useAuthorizedFetch()
  const [busy, setBusy] = useState(false)
  const [confirm, setConfirm] = useState<AdminOrderStatus | "">("")

  async function apply(status: AdminOrderStatus) {
    setBusy(true)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`/api/v1/orders/${encodeURIComponent(orderId)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({ status }),
      })
      if (!res.ok) throw new Error(await readErrorMessage(res))
      await onDone()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Statusänderung fehlgeschlagen")
    } finally {
      setBusy(false)
      setConfirm("")
    }
  }

  return (
    <div className="flex flex-wrap gap-2 pt-1">
      {PENDING_TRANSITIONS.map((s) => (
        <Button
          key={s}
          size="sm"
          variant={s === "cancelled" ? "destructive" : "default"}
          disabled={busy}
          onClick={() => setConfirm(s)}
        >
          {ORDER_STATUS_LABELS[s] || s}
        </Button>
      ))}
      <AlertDialog open={confirm !== ""} onOpenChange={(o) => !o && setConfirm("")}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Status ändern</AlertDialogTitle>
            <AlertDialogDescription>
              Bestellung wirklich auf «{confirm ? ORDER_STATUS_LABELS[confirm] : ""}» setzen?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              disabled={busy}
              onClick={(e) => {
                e.preventDefault()
                if (confirm) void apply(confirm)
              }}
            >
              {busy ? "Wird geändert…" : "Bestätigen"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
