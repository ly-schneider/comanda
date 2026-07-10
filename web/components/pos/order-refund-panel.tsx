"use client"

import { ArrowLeft, Minus, Plus, Printer, ScanLine, Search } from "lucide-react"
import { useCallback, useEffect, useMemo, useState } from "react"
import { OrderScanner } from "@/components/pos/order-scanner"
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
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { useBranding } from "@/contexts/branding-context"
import {
  cancelPosOrder,
  createPosDepositReturn,
  createPosLineRefund,
  getPosRefundView,
  lookupPosOrders,
} from "@/lib/api/pos-refunds"
import { canPrintReceipt, printReceipt, type ReceiptBranding } from "@/lib/pos/receipt"
import { planRefundReceipts, type RefundAction, type RefundReceiptPlan } from "@/lib/pos/refund-receipts"
import {
  ORDER_STATUS_COLORS,
  ORDER_STATUS_LABELS,
  REFUND_KIND_LABELS,
  REFUND_METHOD_LABELS,
  REFUND_STATUS_COLORS,
  REFUND_STATUS_LABELS,
} from "@/lib/refund-format"
import { parseScan } from "@/lib/station-scan"
import { formatChf } from "@/lib/utils"
import type {
  AdminOrder,
  AdminOrderLine,
  AdminOrderStatus,
  Refund,
  RefundView,
  RefundViewDeposit,
} from "@/types/refund"

const REFUNDABLE_STATUSES = new Set<AdminOrderStatus>(["paid", "partially_refunded"])

type RefundedHandler = (action: RefundAction, refund: Refund) => Promise<void>

export function OrderRefundPanel() {
  const { displayName, logoUrl, legal, currency } = useBranding()
  const branding: ReceiptBranding = useMemo(
    () => ({ displayName, logoUrl, address: legal.address, currency }),
    [displayName, logoUrl, legal.address, currency]
  )

  const [query, setQuery] = useState("")
  const [results, setResults] = useState<AdminOrder[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState<string | null>(null)
  const [scannerOpen, setScannerOpen] = useState(false)
  const [scannedToken, setScannedToken] = useState<string | null>(null)

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [view, setView] = useState<RefundView | null>(null)
  const [loadingView, setLoadingView] = useState(false)
  const [viewError, setViewError] = useState<string | null>(null)

  const [result, setResult] = useState<{ plan: RefundReceiptPlan; refund: Refund; action: RefundAction } | null>(null)

  const openOrder = useCallback(async (orderId: string) => {
    setSelectedId(orderId)
    setView(null)
    setViewError(null)
    setLoadingView(true)
    try {
      setView(await getPosRefundView(orderId))
    } catch (e: unknown) {
      setViewError(e instanceof Error ? e.message : "Bestellung konnte nicht geladen werden")
    } finally {
      setLoadingView(false)
    }
  }, [])

  useEffect(() => {
    const q = query.trim()
    if (selectedId) return
    if (q.length < 4) {
      setResults([])
      setSearchError(null)
      setSearching(false)
      return
    }
    let cancelled = false
    setSearching(true)
    const t = window.setTimeout(async () => {
      try {
        const r = await lookupPosOrders(q)
        if (!cancelled) {
          setResults(r.items)
          setSearchError(null)
        }
      } catch (e: unknown) {
        if (!cancelled) {
          setResults([])
          setSearchError(e instanceof Error ? e.message : "Suche fehlgeschlagen")
        }
      } finally {
        if (!cancelled) setSearching(false)
      }
    }, 300)
    return () => {
      cancelled = true
      window.clearTimeout(t)
    }
  }, [query, selectedId])

  const handleScan = useCallback(
    async (result: { text: string; bytes: Uint8Array }) => {
      setScannerOpen(false)
      const parsed = parseScan(result.text, result.bytes)
      if (!parsed || parsed.kind !== "order") {
        setSearchError("Kein gültiger Bestell-Code.")
        return
      }
      setScannedToken(parsed.token)
      setSearching(true)
      setSearchError(null)
      try {
        const r = await lookupPosOrders(parsed.token)
        if (r.items.length === 1) {
          setResults([])
          await openOrder(r.items[0]!.id)
        } else if (r.items.length === 0) {
          setSearchError("Keine Bestellung zu diesem Code gefunden.")
        } else {
          setResults(r.items)
        }
      } catch (e: unknown) {
        setSearchError(e instanceof Error ? e.message : "Suche fehlgeschlagen")
      } finally {
        setSearching(false)
      }
    },
    [openOrder]
  )

  const backToSearch = useCallback(() => {
    setSelectedId(null)
    setView(null)
    setViewError(null)
    setScannedToken(null)
  }, [])

  const handleRefunded = useCallback<RefundedHandler>(
    async (action, refund) => {
      const previousQr = view?.order.qrPayload ?? null
      let fresh: RefundView | null = null
      if (selectedId) {
        try {
          fresh = await getPosRefundView(selectedId)
          setView(fresh)
        } catch {}
      }
      const v = fresh ?? view
      const plan = v
        ? planRefundReceipts({ action, view: v, previousQr, branding })
        : { slips: [], collectOldReceipt: action !== "deposit" }
      if (canPrintReceipt()) plan.slips.forEach(printReceipt)
      setResult({ plan, refund, action })
    },
    [selectedId, view, branding]
  )

  const order = view?.order ?? null
  const canRefund = order ? REFUNDABLE_STATUSES.has(order.status) : false

  return (
    <div className="mx-auto h-full w-full max-w-2xl overflow-y-auto p-4">
      {!selectedId ? (
        <FindOrder
          query={query}
          onQuery={(q) => {
            setQuery(q)
            setScannedToken(null)
          }}
          results={results}
          searching={searching}
          error={searchError}
          onScanClick={() => {
            setSearchError(null)
            setScannerOpen(true)
          }}
          onOpen={openOrder}
        />
      ) : (
        <div className="space-y-5">
          <Button variant="ghost" size="sm" className="-ml-2" onClick={backToSearch}>
            <ArrowLeft className="mr-1 size-4" />
            Andere Bestellung
          </Button>

          {loadingView && <p className="text-muted-foreground text-sm">Lade Bestellung…</p>}
          {viewError && <p className="text-sm text-red-600">{viewError}</p>}

          {view && order && (
            <>
              <OrderHeader order={order} refundableBalanceCents={view.refundableBalanceCents} />

              <ItemOverview view={view} />

              {canRefund ? (
                <div className="space-y-6 rounded-xl border p-4">
                  <h2 className="text-base font-semibold">Aktionen</h2>
                  <PosLineRefundSection orderId={order.id} view={view} onRefunded={handleRefunded} />
                  <PosDepositReturnSection
                    orderId={order.id}
                    deposits={view.deposits}
                    token={scannedToken}
                    onRefunded={handleRefunded}
                  />
                  <Separator />
                  <PosCancelSection orderId={order.id} onRefunded={handleRefunded} />
                </div>
              ) : (
                <p className="text-muted-foreground rounded-xl border p-4 text-sm">
                  Diese Bestellung kann nicht erstattet werden.
                </p>
              )}

              {view.refunds.length > 0 && <RefundHistory refunds={view.refunds} />}
            </>
          )}
        </div>
      )}

      <OrderScanner open={scannerOpen} onOpenChange={setScannerOpen} onScan={handleScan} />

      <ResultDialog result={result} onClose={() => setResult(null)} />
    </div>
  )
}

function FindOrder({
  query,
  onQuery,
  results,
  searching,
  error,
  onScanClick,
  onOpen,
}: {
  query: string
  onQuery: (q: string) => void
  results: AdminOrder[]
  searching: boolean
  error: string | null
  onScanClick: () => void
  onOpen: (orderId: string) => void
}) {
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold">Bestellung suchen</h1>
        <p className="text-muted-foreground text-sm">Bestell-ID eingeben oder den Abhol-QR scannen.</p>
      </div>
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => onQuery(e.target.value)}
            placeholder="Erste Zeichen der Bestell-ID"
            className="pl-9"
            inputMode="text"
            autoComplete="off"
          />
        </div>
        <Button variant="outline" onClick={onScanClick}>
          <ScanLine className="mr-2 size-4" />
          Scannen
        </Button>
      </div>

      {error && <p className="text-sm text-red-600">{error}</p>}
      {searching && <p className="text-muted-foreground text-sm">Suche läuft…</p>}
      {!searching && query.trim().length >= 4 && results.length === 0 && !error && (
        <p className="text-muted-foreground text-sm">Keine Bestellungen gefunden.</p>
      )}

      <div className="flex flex-col gap-2">
        {results.map((o) => (
          <button
            key={o.id}
            onClick={() => onOpen(o.id)}
            className="hover:bg-muted/50 flex items-center justify-between gap-3 rounded-xl border p-3 text-left transition-colors"
          >
            <div className="min-w-0">
              <code className="text-sm font-medium">{o.id}</code>
              <p className="text-muted-foreground text-xs">{new Date(o.createdAt).toLocaleString("de-CH")}</p>
            </div>
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium">{formatChf(o.totalCents)}</span>
              <StatusBadge status={o.status} />
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}

function StatusBadge({ status }: { status: AdminOrderStatus }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${ORDER_STATUS_COLORS[status] || "bg-gray-100 text-gray-800"}`}
    >
      {ORDER_STATUS_LABELS[status] || status}
    </span>
  )
}

function OrderHeader({ order, refundableBalanceCents }: { order: AdminOrder; refundableBalanceCents: number }) {
  const payment = order.payments?.[0]
  const method = payment?.method
  const cardLabel = [payment?.cardBrand, payment?.cardLast4 ? `•••• ${payment.cardLast4}` : null]
    .filter(Boolean)
    .join(" ")
  return (
    <div className="rounded-xl border p-4">
      <div className="flex items-center justify-between gap-3">
        <code className="text-sm font-medium">{order.id}</code>
        <StatusBadge status={order.status} />
      </div>
      <div className="text-muted-foreground mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
        <span>Summe</span>
        <span className="text-foreground text-right font-medium">{formatChf(order.totalCents)}</span>
        <span>Erstattbar</span>
        <span className="text-foreground text-right font-medium">{formatChf(refundableBalanceCents)}</span>
        {method && (
          <>
            <span>Zahlart</span>
            <span className="text-foreground text-right uppercase">{method}</span>
          </>
        )}
        {cardLabel && (
          <>
            <span>Karte</span>
            <span className="text-foreground text-right">{cardLabel}</span>
          </>
        )}
        {payment?.cardTransactionId && (
          <>
            <span>Trans.-Nr.</span>
            <span className="text-foreground text-right font-mono text-xs break-all">{payment.cardTransactionId}</span>
          </>
        )}
        <span>Erstellt</span>
        <span className="text-foreground text-right">{new Date(order.createdAt).toLocaleString("de-CH")}</span>
      </div>
    </div>
  )
}

function ItemOverview({ view }: { view: RefundView }) {
  if (view.lines.length === 0) return null
  return (
    <div className="rounded-xl border p-4">
      <h2 className="mb-2 text-base font-semibold">Artikel</h2>
      <div className="flex flex-col gap-2">
        {view.lines.map((l) => (
          <div key={l.orderLineId} className="flex items-center justify-between gap-3 text-sm">
            <div className="flex min-w-0 items-center gap-2">
              <span className="truncate font-medium">
                {l.title}
                {l.variantValue ? ` (${l.variantValue})` : ""}
              </span>
              {l.redeemed && (
                <span className="inline-flex shrink-0 rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-800">
                  Bereits eingelöst
                </span>
              )}
            </div>
            <span className="text-muted-foreground shrink-0">
              x{l.boughtQuantity}
              {l.refundedQuantity > 0 ? ` · ${l.refundedQuantity} erstattet` : ""}
            </span>
          </div>
        ))}
      </div>
    </div>
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
        className="h-9 w-9"
        disabled={disabled || value <= 0}
        onClick={() => onChange(Math.max(0, value - 1))}
        aria-label="Weniger"
      >
        <Minus className="size-4" />
      </Button>
      <span className="w-6 text-center text-sm tabular-nums">{value}</span>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="h-9 w-9"
        disabled={disabled || value >= max}
        onClick={() => onChange(Math.min(max, value + 1))}
        aria-label="Mehr"
      >
        <Plus className="size-4" />
      </Button>
    </div>
  )
}

function PosLineRefundSection({
  orderId,
  view,
  onRefunded,
}: {
  orderId: string
  view: RefundView
  onRefunded: RefundedHandler
}) {
  const [qty, setQty] = useState<Record<string, number>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refundable = view.lines.filter((l) => l.refundableQuantity > 0)
  const orderLines = useMemo(() => view.order.lines ?? [], [view.order.lines])

  // Each product line auto-refunds its linked deposit (capped at outstanding).
  const depositByParent = useMemo(() => {
    const m: Record<string, AdminOrderLine> = {}
    for (const l of orderLines) if (l.lineType === "deposit" && l.parentLineId) m[l.parentLineId] = l
    return m
  }, [orderLines])
  const outstandingByDeposit = useMemo(() => {
    const m: Record<string, number> = {}
    for (const d of view.deposits) m[d.orderLineId] = d.outstandingQuantity
    return m
  }, [view.deposits])

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
      const refund = await createPosLineRefund(
        orderId,
        selected.map((x) => ({ orderLineId: x.line.orderLineId, quantity: x.q }))
      )
      setQty({})
      await onRefunded("line", refund)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Rückerstattung fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold">Produkt erstatten</h3>
      <div className="flex flex-col gap-2">
        {refundable.map((l) => {
          const dep = depositByParent[l.orderLineId]
          return (
            <div key={l.orderLineId} className="flex items-center justify-between gap-3 rounded-lg border p-2">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium">
                    {l.title}
                    {l.variantValue ? ` (${l.variantValue})` : ""}
                  </span>
                  {l.redeemed && (
                    <span className="inline-flex shrink-0 rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-800">
                      Bereits eingelöst
                    </span>
                  )}
                </div>
                <p className="text-muted-foreground text-xs">
                  {formatChf(l.unitPriceCents)} · noch {l.refundableQuantity} von {l.boughtQuantity}
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
          Betrag: <span className="font-medium">{formatChf(productCents + depositCents)}</span>
          {depositCents > 0 && <span className="text-muted-foreground"> (inkl. Depot {formatChf(depositCents)})</span>}
        </span>
        <Button size="sm" disabled={busy || selected.length === 0} onClick={() => void submit()}>
          {busy ? "Wird erstattet…" : "Erstatten"}
        </Button>
      </div>
    </div>
  )
}

function PosDepositReturnSection({
  orderId,
  deposits,
  token,
  onRefunded,
}: {
  orderId: string
  deposits: RefundViewDeposit[]
  token: string | null
  onRefunded: RefundedHandler
}) {
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
      const refund = await createPosDepositReturn(
        orderId,
        selected.map((x) => ({ orderLineId: x.deposit.orderLineId, quantity: x.q })),
        token
      )
      setQty({})
      await onRefunded("deposit", refund)
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
          <div key={d.orderLineId} className="flex items-center justify-between gap-3 rounded-lg border p-2">
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

function PosCancelSection({ orderId, onRefunded }: { orderId: string; onRefunded: RefundedHandler }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    setBusy(true)
    setError(null)
    try {
      const refund = await cancelPosOrder(orderId)
      setOpen(false)
      await onRefunded("cancel", refund)
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
          <p className="text-muted-foreground text-xs">Erstattet den gesamten Restbetrag und entwertet den Abhol-QR.</p>
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
              className="bg-destructive hover:bg-destructive-hover text-white"
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
      <h2 className="text-base font-semibold">Rückerstattungen</h2>
      <div className="flex flex-col gap-2">
        {refunds.map((r) => (
          <div key={r.id} className="flex items-start justify-between gap-3 rounded-lg border p-3 text-sm">
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

function ResultDialog({
  result,
  onClose,
}: {
  result: { plan: RefundReceiptPlan; refund: Refund; action: RefundAction } | null
  onClose: () => void
}) {
  if (!result) return null
  const { plan, refund, action } = result
  const isCancel = action === "cancel"
  const isTwint = refund.method === "twint"
  const printable = canPrintReceipt()

  return (
    <Dialog open={!!result} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isCancel ? "Bestellung storniert" : "Rückerstattung abgeschlossen"}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="bg-muted rounded-lg p-3">
            <div className="text-muted-foreground text-xs">Betrag</div>
            <div className="text-lg font-semibold">{formatChf(refund.amountCents)}</div>
            <p className="mt-1 text-sm">
              {isTwint
                ? "Wird automatisch an TWINT zurückerstattet."
                : `${formatChf(refund.amountCents)} in bar an die Kundschaft zurückgeben.`}
            </p>
          </div>

          {plan.collectOldReceipt && (
            <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
              Alten Beleg einziehen.
            </div>
          )}

          {plan.slips.length > 0 &&
            (printable ? (
              <p className="text-muted-foreground text-sm">
                {plan.slips.length > 1 ? "Belege werden gedruckt." : "Beleg wird gedruckt."}
              </p>
            ) : (
              <p className="text-muted-foreground text-sm">Drucken ist nur in der POS-App verfügbar.</p>
            ))}
        </div>
        <DialogFooter className="gap-2">
          {plan.slips.length > 0 && (
            <Button variant="outline" disabled={!printable} onClick={() => plan.slips.forEach(printReceipt)}>
              <Printer className="mr-2 size-4" />
              Erneut drucken
            </Button>
          )}
          <Button onClick={onClose}>Fertig</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
