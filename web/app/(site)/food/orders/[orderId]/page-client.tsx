"use client"

import { ArrowLeft, Check } from "lucide-react"
import Image from "next/image"
import { useParams, useRouter, useSearchParams } from "next/navigation"
import { useEffect, useMemo, useRef, useState } from "react"
import QRCode from "@/components/qrcode"
import { Button } from "@/components/ui/button"
import { useCart } from "@/contexts/cart-context"

import { getOrderPublicById, type OrderLineDTO, type PublicOrderDetailsDTO } from "@/lib/api/orders"
import { addOrder } from "@/lib/orders-storage"
import { ORDER_STATUS_COLORS, ORDER_STATUS_LABELS } from "@/lib/refund-format"
import { formatChf } from "@/lib/utils"

function paymentMethodLabel(method: string | undefined): string {
  switch (method) {
    case "CASH":
      return "Bar"
    case "CARD":
      return "Karte"
    case "TWINT":
      return "TWINT"
    default:
      return method ?? "–"
  }
}

function entryModeLabel(mode: string): string {
  const m = mode.toLowerCase()
  if (m.includes("contact")) return "Kontaktlos"
  if (m.includes("chip")) return "Chip"
  if (m.includes("swipe") || m.includes("magnet")) return "Magnetstreifen"
  return mode
}

export default function OrderPageClient() {
  const sp = useSearchParams()
  const { orderId } = useParams<{ orderId: string }>()
  const from = sp.get("from")
  const { clearCart } = useCart()
  const clearedRef = useRef(false)
  const footerRef = useRef<HTMLDivElement>(null)
  const [footerHeight, setFooterHeight] = useState(0)
  const [mounted, setMounted] = useState(false)
  const [serverOrder, setServerOrder] = useState<PublicOrderDetailsDTO | null>(null)
  const [loading, setLoading] = useState<boolean>(false)
  const [apiError, setApiError] = useState<string | null>(null)
  const [qrReady, setQrReady] = useState<boolean>(false)
  const initialLoadDone = useRef(false)

  useEffect(() => {
    if (orderId) addOrder(orderId)
  }, [orderId])

  // Load order details from API (public) for any order id
  useEffect(() => {
    let cancelled = false
    async function load() {
      if (!orderId) return
      if (!initialLoadDone.current) {
        setQrReady(false)
        setLoading(true)
      }
      setApiError(null)
      try {
        const res = await getOrderPublicById(orderId)
        if (!cancelled) setServerOrder(res)
      } catch (e: unknown) {
        const msg =
          typeof e === "object" && e && "message" in e ? String((e as { message?: unknown }).message ?? "") : undefined
        if (!cancelled) setApiError(msg || "Fehler beim Laden der Bestellung")
      } finally {
        if (!cancelled) {
          initialLoadDone.current = true
          setQrReady(true)
          setLoading(false)
        }
      }
    }
    void load()
    const interval = setInterval(load, 10_000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [orderId])

  // Ensure cart is cleared when landing on QR page as a fallback
  useEffect(() => {
    if (!clearedRef.current) {
      clearedRef.current = true
      clearCart()
    }
  }, [clearCart])
  // Avoid hydration mismatches by rendering dynamic content only after mount
  useEffect(() => {
    setMounted(true)
  }, [])
  const router = useRouter()

  // Measure footer height to ensure content (e.g., Summe) is not obscured
  useEffect(() => {
    const el = footerRef.current
    if (!el) return
    const update = () => setFooterHeight(el.offsetHeight || 0)
    update()
    let ro: ResizeObserver | null = null
    if (typeof ResizeObserver !== "undefined") {
      ro = new ResizeObserver(() => update())
      ro.observe(el)
    }
    const onResize = () => update()
    window.addEventListener("resize", onResize)
    return () => {
      window.removeEventListener("resize", onResize)
      if (ro) ro.disconnect()
    }
  }, [from, mounted])

  const groupedItems = useMemo(() => {
    if (!serverOrder?.lines || serverOrder.lines.length === 0)
      return [] as Array<{ parent: OrderLineDTO; children: OrderLineDTO[] }>

    const byId: Record<string, OrderLineDTO> = {}
    for (const it of serverOrder.lines) byId[it.id] = it

    const childrenByRoot: Record<string, OrderLineDTO[]> = {}
    const roots: OrderLineDTO[] = []

    for (const it of serverOrder.lines) {
      const hasParent = typeof it.parentLineId === "string" && it.parentLineId.length > 0
      if (!hasParent) {
        roots.push(it)
        continue
      }
      let parentId = it.parentLineId as string
      while (parentId) {
        const node = byId[parentId]
        if (!node) break
        const p = node.parentLineId
        if (typeof p === "string" && p.length > 0) {
          parentId = p
        } else {
          break
        }
      }
      const arr = childrenByRoot[parentId] || []
      arr.push(it)
      childrenByRoot[parentId] = arr
    }

    return roots.map((root) => ({ parent: root, children: childrenByRoot[root.id] || [] }))
  }, [serverOrder])

  const status = serverOrder?.status
  // A cancelled or fully refunded order has nothing left to hand over, so the
  // backend clears/empties the pickup token and we show no QR.
  const pickupSuppressed = status === "cancelled" || status === "refunded"
  const refundedTotalCents = serverOrder?.refundedTotalCents ?? 0

  const qrDisplay = (() => {
    if (!mounted) {
      return <div className="mx-auto h-[260px] w-[260px] animate-pulse rounded-[11px] border-2 bg-gray-100" />
    }
    if (!orderId) {
      return <p className="text-red-600">Bestellnummer fehlt.</p>
    }
    if (qrReady) {
      if (pickupSuppressed) {
        return (
          <div className="mx-auto flex h-[260px] w-[260px] flex-col items-center justify-center rounded-[11px] border-2 border-dashed bg-gray-50 px-6 text-center">
            <p className="text-muted-foreground text-sm">
              {status === "cancelled"
                ? "Diese Bestellung wurde storniert. Es gibt keinen Abhol-Code."
                : "Diese Bestellung wurde vollständig erstattet. Es gibt keinen Abhol-Code."}
            </p>
          </div>
        )
      }
      // Signed token → binary v2 QR (byte-mode); the plain pickup URL is the text fallback.
      if (serverOrder?.qrPayload) {
        return (
          <QRCode value={serverOrder.qrPayload} binary size={260} className="mx-auto rounded-[11px] border-2 p-1" />
        )
      }
      return (
        <QRCode
          value={`${window.location.origin}/o/${orderId}`}
          size={260}
          className="mx-auto rounded-[11px] border-2 p-1"
        />
      )
    }
    return <div className="mx-auto h-[260px] w-[260px] animate-pulse rounded-[11px] border-2 bg-gray-100" />
  })()

  return (
    <div className="flex flex-col p-4" style={{ paddingBottom: footerHeight ? footerHeight + 16 : 16 }}>
      <h1 className="mb-2 text-2xl font-semibold">{pickupSuppressed ? "Deine Bestellung" : "Dein Abhol-QR-Code"}</h1>
      {mounted && status && status !== "paid" && status !== "pending" && (
        <div className="mb-3">
          <span className={`inline-flex rounded-full px-3 py-1 text-sm font-medium ${ORDER_STATUS_COLORS[status]}`}>
            {ORDER_STATUS_LABELS[status]}
          </span>
        </div>
      )}
      {!pickupSuppressed && (
        <>
          <p className={`text-muted-foreground text-sm ${from === "success" ? "mb-2" : "mb-6"}`}>
            Zeigen Sie diesen QR-Code bei der Abholung vor.
          </p>
          {from === "success" && (
            <p className="text-muted-foreground mb-6 text-sm">
              Du kannst diesen QR-Code jederzeit in deinen Bestellungen finden.
            </p>
          )}
        </>
      )}
      {qrDisplay}

      {/* Order items summary */}
      {mounted && serverOrder && groupedItems.length > 0 ? (
        <div className="mt-8">
          <h2 className="mb-3 text-lg font-semibold">Bestellte Artikel</h2>
          <div className="flex flex-col gap-3">
            {groupedItems.map(({ parent, children }) => {
              const components = children.filter((c) => c.lineType === "component")
              const deposits = children.filter((c) => c.lineType === "deposit")
              const isRedeemed =
                parent.redemption != null || (components.length > 0 && components.every((c) => c.redemption != null))
              const refundedQty = parent.refundedQuantity ?? 0
              const remainingQty = Math.max(parent.quantity - refundedQty, 0)
              const fullyRefunded = refundedQty > 0 && remainingQty === 0
              return (
                <div key={parent.id} className={`rounded-xl border p-3 ${fullyRefunded ? "opacity-60" : ""}`}>
                  <div className="flex items-center gap-3">
                    {parent.productImage && (
                      <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-[11px] bg-[#cec9c6]">
                        <Image
                          src={parent.productImage}
                          alt={"Produktbild von " + parent.title}
                          fill
                          sizes="128px"
                          quality={90}
                          className="h-full w-full rounded-[11px] object-cover"
                        />
                      </div>
                    )}
                    <div className="min-w-0 flex-1">
                      <div className={`flex justify-between gap-3 ${isRedeemed ? "items-start" : "items-center"}`}>
                        <div className="min-w-0">
                          <p className={`truncate font-medium ${fullyRefunded ? "line-through" : ""}`}>
                            {parent.title}
                            {parent.variantValue ? ` (${parent.variantValue})` : ""}
                          </p>
                          {parent.productDescription && (
                            <p className="text-muted-foreground mt-0.5 line-clamp-2 text-xs whitespace-pre-line">
                              {parent.productDescription}
                            </p>
                          )}
                          {components.length > 0 && (
                            <div className="mt-1 flex flex-row flex-wrap gap-1.5">
                              {components.map((c) => (
                                <span
                                  key={c.id}
                                  className="text-muted-foreground border-border rounded-lg border px-2 py-0.5 text-xs"
                                >
                                  {c.menuSlotName ?? "Option"}: {c.title}
                                </span>
                              ))}
                            </div>
                          )}
                          {deposits.map((d) => {
                            const dReturned = d.refundedQuantity ?? 0
                            const dRemaining = Math.max(d.quantity - dReturned, 0)
                            return (
                              <p key={d.id} className="text-muted-foreground mt-1 text-xs">
                                {dRemaining > 0
                                  ? `Depot ×${dRemaining} · ${formatChf(d.unitPriceCents * dRemaining)}`
                                  : "Depot zurückerstattet"}
                                {dReturned > 0 && dRemaining > 0 ? ` · ${dReturned}× zurückerstattet` : ""}
                              </p>
                            )
                          })}
                          {refundedQty > 0 && (
                            <div className="mt-1.5 inline-flex items-center gap-1 rounded-md bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800">
                              {fullyRefunded ? "Erstattet" : `${refundedQty}× erstattet`}
                            </div>
                          )}
                          {isRedeemed && (
                            <div className="mt-1.5 inline-flex items-center gap-1 rounded-md bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-800">
                              <Check className="size-3" />
                              Abgeholt
                            </div>
                          )}
                        </div>
                        {!fullyRefunded && (
                          <div className="shrink-0 text-right">
                            <p className="text-muted-foreground text-sm">x{remainingQty}</p>
                            <p className="font-medium">{formatChf(parent.unitPriceCents * remainingQty)}</p>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
          {typeof serverOrder.totalCents === "number" && (
            <div className="mt-4 flex flex-col gap-1">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground text-sm">Summe</span>
                <span className="text-base font-semibold">{formatChf(serverOrder.totalCents)}</span>
              </div>
              {refundedTotalCents > 0 && (
                <>
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">Erstattet</span>
                    <span className="text-blue-700">−{formatChf(refundedTotalCents)}</span>
                  </div>
                  <div className="flex items-center justify-between border-t pt-1">
                    <span className="text-muted-foreground text-sm">Verbleibend</span>
                    <span className="text-base font-semibold">
                      {formatChf(Math.max(serverOrder.totalCents - refundedTotalCents, 0))}
                    </span>
                  </div>
                </>
              )}
            </div>
          )}

          {serverOrder.payments && serverOrder.payments.length > 0 && (
            <div className="mt-6">
              <h2 className="mb-3 text-lg font-semibold">Zahlung</h2>
              <div className="rounded-xl border p-3 text-sm">
                {serverOrder.payments.map((p) => (
                  <div key={p.id ?? p.paidAt ?? ""} className="flex flex-col gap-1">
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">Methode</span>
                      <span>{paymentMethodLabel(p.method)}</span>
                    </div>
                    {p.paidAt && (
                      <div className="flex items-center justify-between">
                        <span className="text-muted-foreground">Bezahlt am</span>
                        <span>{new Date(p.paidAt).toLocaleString("de-CH")}</span>
                      </div>
                    )}
                    {(p.cardBrand || p.cardLast4) && (
                      <div className="flex items-center justify-between">
                        <span className="text-muted-foreground">Karte</span>
                        <span>
                          {[p.cardBrand, p.cardLast4 ? `•••• ${p.cardLast4}` : null].filter(Boolean).join(" ")}
                        </span>
                      </div>
                    )}
                    {p.entryMode && (
                      <div className="flex items-center justify-between">
                        <span className="text-muted-foreground">Einlesemodus</span>
                        <span>{entryModeLabel(p.entryMode)}</span>
                      </div>
                    )}
                    {p.cardTransactionId && (
                      <div className="flex items-center justify-between">
                        <span className="text-muted-foreground">Transaktion</span>
                        <span className="font-mono text-xs">{p.cardTransactionId}</span>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="mt-6 rounded-xl border p-3 text-sm">
            <p className="font-semibold">Comanda</p>
            <p className="text-muted-foreground">Industriestrasse 5, 3600 Thun</p>
            <p className="text-muted-foreground mt-2">Fragen zur Bestellung? Sprich uns an der Kasse an.</p>
            <p className="text-muted-foreground mt-2 text-xs">
              <a className="underline" href="/nutzungsbedingungen">
                Nutzungsbedingungen
              </a>
              {" · "}
              <a className="underline" href="/datenschutz">
                Datenschutz
              </a>
            </p>
          </div>
        </div>
      ) : mounted && orderId ? (
        <div className="mt-8">
          <h2 className="mb-3 text-lg font-semibold">Bestellte Artikel</h2>
          {loading ? (
            <p className="text-muted-foreground text-sm">Lade Bestellung…</p>
          ) : apiError ? (
            <p className="text-sm text-red-600">{apiError}</p>
          ) : (
            <p className="text-muted-foreground text-sm">
              Keine Artikelliste gespeichert. Der QR-Code identifiziert die Bestellung beim Abholen.
            </p>
          )}
        </div>
      ) : null}

      <div ref={footerRef} className="fixed inset-x-0 bottom-0 z-50 mx-auto max-w-xl p-4">
        <div className="flex flex-col gap-2">
          <Button
            variant="outline"
            className="rounded-pill h-12 w-full bg-white text-base font-medium text-black"
            onClick={() => router.push(from === "orders" ? "/food/orders" : "/")}
          >
            <ArrowLeft style={{ width: 20, height: 20 }} /> {from === "orders" ? "Zurück" : "Zum Menü"}
          </Button>

          {from === "success" && (
            <Button
              variant="selected"
              className="rounded-pill h-12 w-full text-base"
              onClick={() => router.push("/food/orders")}
            >
              Alle Bestellungen
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
