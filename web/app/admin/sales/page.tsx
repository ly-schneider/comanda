"use client"
import { Download } from "lucide-react"
import { useCallback, useEffect, useMemo, useState } from "react"
import type { DateRange } from "react-day-picker"
import { DateRangePicker } from "@/components/admin/date-range-picker"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { rangeBounds } from "@/lib/inventory-export"
import { formatChf } from "@/lib/utils"
import { StatCard } from "../_components/stat-card"

type Totals = {
  orderCount: number
  itemCount: number
  revenueCents: number
  depositCents: number
  refundedCents: number
  netCents: number
  gratisCents: number
  gratisOrderCount: number
  gratisItemCount: number
}

type PaymentMethod = {
  method: string
  orderCount: number
  revenueCents: number
  depositCents: number
  refundedCents: number
  netCents: number
  gratisCents: number
}

type Article = {
  category?: string | null
  subcategory?: string | null
  product: string
  variant?: string | null
  quantity: number
  bundleQty: number
  gratisQty: number
  refundedQty: number
  revenueCents: number
  depositCents: number
  refundedCents: number
  netCents: number
  gratisCents: number
}

type Bundle = {
  product: string
  quantity: number
  gratisQty: number
  refundedQty: number
  orderCount: number
  revenueCents: number
  depositCents: number
  refundedCents: number
  netCents: number
  gratisCents: number
}

type Summary = { totals: Totals; methods: PaymentMethod[]; articles: Article[]; bundles: Bundle[] }

type EventDay = { year: number; month: number; day: number }

function eventDayValue(day: EventDay) {
  return `${day.year}-${day.month}-${day.day}`
}

function rangeParams(range: DateRange | undefined): URLSearchParams | null {
  if (!range) return null
  const bounds = rangeBounds(range)
  if (!bounds) return null
  return new URLSearchParams({ from: bounds.from.toISOString(), to: bounds.to.toISOString() })
}

export default function SalesPage() {
  const fetchAuth = useAuthorizedFetch()
  const [events, setEvents] = useState<EventDay[]>([])
  const [range, setRange] = useState<DateRange | undefined>()
  const [summary, setSummary] = useState<Summary | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetchAuth("/api/v1/events")
        if (!res.ok) return
        const data = (await res.json()) as { items?: EventDay[] }
        if (!cancelled) setEvents(data.items ?? [])
      } catch {
        // Leave the selector empty.
      }
    })()
    return () => {
      cancelled = true
    }
  }, [fetchAuth])

  const params = useMemo(() => rangeParams(range), [range])

  const loadSummary = useCallback(
    async (query: URLSearchParams) => {
      setLoading(true)
      setError(null)
      try {
        const res = await fetchAuth(`/api/v1/sales/summary?${query}`)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        setSummary((await res.json()) as Summary)
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : "Fehler beim Laden")
        setSummary(null)
      } finally {
        setLoading(false)
      }
    },
    [fetchAuth]
  )

  useEffect(() => {
    if (!params) {
      setSummary(null)
      return
    }
    void loadSummary(params)
  }, [params, loadSummary])

  function selectEventDay(value: string) {
    const day = events.find((e) => eventDayValue(e) === value)
    if (!day) return
    const picked = new Date(day.year, day.month - 1, day.day)
    setRange({ from: picked, to: picked })
  }

  // Not fetch: apiRequest always parses JSON, and window.open carries the session cookie.
  function downloadExport() {
    if (!params) return
    window.open(`/api/v1/sales/export.xlsx?${params}`, "_blank")
  }

  const totals = summary?.totals
  const articles = summary?.articles ?? []
  const bundles = summary?.bundles ?? []
  const showVariant = articles.some((a) => a.variant)
  const showBundle = articles.some((a) => a.bundleQty > 0)

  return (
    <div className="min-w-0 space-y-6">
      <h1 className="text-xl font-semibold">Verkäufe & Umsatz</h1>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor="sales-range">Zeitraum</Label>
          <DateRangePicker id="sales-range" value={range} onChange={setRange} />
        </div>

        <div className="space-y-3">
          {events.length > 0 && (
            <div className="space-y-1">
              <Label htmlFor="sales-event">Verkaufstag übernehmen</Label>
              <Select onValueChange={selectEventDay}>
                <SelectTrigger id="sales-event" className="h-10">
                  <SelectValue placeholder="Tag wählen" />
                </SelectTrigger>
                <SelectContent>
                  {events.map((e) => (
                    <SelectItem key={eventDayValue(e)} value={eventDayValue(e)}>
                      {new Date(e.year, e.month - 1, e.day).toLocaleDateString("de-CH", { dateStyle: "medium" })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          <Button onClick={downloadExport} disabled={!params} className="w-full">
            <Download className="mr-2 size-4" />
            Excel exportieren
          </Button>
        </div>
      </div>

      {error && <div className="text-sm text-red-600">{error}</div>}
      {!params && !loading && <p className="text-muted-foreground text-sm">Wähle einen Zeitraum.</p>}

      {totals && (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard title="Warenumsatz" value={formatChf(totals.revenueCents)} loading={loading} />
            <StatCard title="Nettoeinnahmen" value={formatChf(totals.netCents)} loading={loading} />
            <StatCard title="Bestellungen" value={String(totals.orderCount)} loading={loading} />
            <StatCard
              title="Gratis abgegeben"
              value={formatChf(totals.gratisCents)}
              loading={loading}
              comparison={
                totals.gratisOrderCount > 0
                  ? {
                      label: `${totals.gratisOrderCount} Bestellungen · ${totals.gratisItemCount} Artikel`,
                      positive: false,
                    }
                  : null
              }
            />
          </div>

          <section className="space-y-2">
            <h2 className="text-sm font-semibold">Nach Zahlungsart</h2>
            <TableContainer>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Zahlungsart</TableHead>
                    <TableHead className="text-right">Bestellungen</TableHead>
                    <TableHead className="text-right">Warenumsatz</TableHead>
                    <TableHead className="text-right">Depot</TableHead>
                    <TableHead className="text-right">Erstattet</TableHead>
                    <TableHead className="text-right">Netto</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {summary.methods.map((m) => (
                    <TableRow key={m.method}>
                      <TableCell>{m.method}</TableCell>
                      <TableCell className="text-right">{m.orderCount}</TableCell>
                      <TableCell className="text-right">{formatChf(m.revenueCents)}</TableCell>
                      <TableCell className="text-right">{formatChf(m.depositCents)}</TableCell>
                      <TableCell className="text-right">{formatChf(-m.refundedCents)}</TableCell>
                      <TableCell className="text-right font-medium">{formatChf(m.netCents)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </section>

          {bundles.length > 0 && (
            <section className="space-y-2">
              <h2 className="text-sm font-semibold">Bundles</h2>
              <p className="text-muted-foreground text-sm">
                Dasselbe Geld wie unter «Pro Artikel», nur nach Bundle gezählt statt nach Artikel.
              </p>
              <TableContainer>
                <div
                  className="overflow-x-auto overscroll-x-contain"
                  role="region"
                  aria-label="Bundles, horizontal scrollen für weitere Spalten"
                  tabIndex={0}
                >
                  <Table className="whitespace-nowrap">
                    <TableHeader>
                      <TableRow>
                        <TableHead>Bundle</TableHead>
                        <TableHead className="text-right">Menge</TableHead>
                        <TableHead className="text-right">Bestellungen</TableHead>
                        <TableHead className="text-right">davon gratis</TableHead>
                        <TableHead className="text-right">Warenumsatz</TableHead>
                        <TableHead className="text-right">Erstattet</TableHead>
                        <TableHead className="text-right">Netto</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {bundles.map((b) => (
                        <TableRow key={b.product}>
                          <TableCell>{b.product}</TableCell>
                          <TableCell className="text-right">{b.quantity}</TableCell>
                          <TableCell className="text-right">{b.orderCount}</TableCell>
                          <TableCell className="text-right">{b.gratisQty}</TableCell>
                          <TableCell className="text-right">{formatChf(b.revenueCents)}</TableCell>
                          <TableCell className="text-right">{formatChf(-b.refundedCents)}</TableCell>
                          <TableCell className="text-right font-medium">{formatChf(b.netCents)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </TableContainer>
            </section>
          )}

          <section className="space-y-2">
            <h2 className="text-sm font-semibold">Pro Artikel</h2>
            {articles.length === 0 ? (
              <p className="text-muted-foreground text-sm">Keine Verkäufe in diesem Zeitraum.</p>
            ) : (
              <TableContainer>
                <div
                  className="overflow-x-auto overscroll-x-contain"
                  role="region"
                  aria-label="Artikel, horizontal scrollen für weitere Spalten"
                  tabIndex={0}
                >
                  <Table className="whitespace-nowrap">
                    <TableHeader>
                      <TableRow>
                        <TableHead>Artikel</TableHead>
                        {showVariant && <TableHead>Variante</TableHead>}
                        <TableHead className="text-right">Menge</TableHead>
                        {showBundle && <TableHead className="text-right">davon Bundle</TableHead>}
                        <TableHead className="text-right">davon gratis</TableHead>
                        <TableHead className="text-right">Warenumsatz</TableHead>
                        <TableHead className="text-right">Erstattet</TableHead>
                        <TableHead className="text-right">Netto</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {articles.map((a) => (
                        <TableRow key={`${a.product}-${a.variant ?? ""}-${a.category ?? ""}`}>
                          <TableCell>
                            {a.product}
                            {(a.category || a.subcategory) && (
                              <span className="text-muted-foreground block text-xs">
                                {[a.category, a.subcategory].filter(Boolean).join(" · ")}
                              </span>
                            )}
                          </TableCell>
                          {showVariant && (
                            <TableCell>{a.variant || <span className="text-muted-foreground">–</span>}</TableCell>
                          )}
                          <TableCell className="text-right">{a.quantity}</TableCell>
                          {showBundle && <TableCell className="text-right">{a.bundleQty}</TableCell>}
                          <TableCell className="text-right">{a.gratisQty}</TableCell>
                          <TableCell className="text-right">{formatChf(a.revenueCents)}</TableCell>
                          <TableCell className="text-right">{formatChf(-a.refundedCents)}</TableCell>
                          <TableCell className="text-right font-medium">{formatChf(a.netCents)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </TableContainer>
            )}
          </section>
        </>
      )}

      {loading && <p className="text-muted-foreground text-sm">Lade…</p>}
    </div>
  )
}
