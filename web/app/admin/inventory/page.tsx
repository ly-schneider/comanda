"use client"
import { Download } from "lucide-react"
import Link from "next/link"
import { useCallback, useEffect, useMemo, useState } from "react"
import type { DateRange } from "react-day-picker"
import { CategorySelect } from "@/components/admin/category-select"
import { DateRangePicker } from "@/components/admin/date-range-picker"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import type { CategoryLike } from "@/lib/categories"
import { categoryScope, scopeParams } from "@/lib/inventory-export"

type Product = { id: string; name: string }

type Movement = {
  at: string
  category?: string | null
  subcategory?: string | null
  product: string
  variant?: string | null
  delta: number
  reason: string
  orderId?: string | null
  createdBy?: string | null
  createdById?: string | null
}

type EventDay = { year: number; month: number; day: number }

type ScopeMode = "category" | "product"

const PREVIEW_LIMIT = 200

function eventDayValue(day: EventDay) {
  return `${day.year}-${day.month}-${day.day}`
}

export default function InventoryHistoryPage() {
  const fetchAuth = useAuthorizedFetch()
  const [categories, setCategories] = useState<CategoryLike[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [events, setEvents] = useState<EventDay[]>([])

  const [mode, setMode] = useState<ScopeMode>("category")
  const [categoryId, setCategoryId] = useState("")
  const [productId, setProductId] = useState("")
  const [range, setRange] = useState<DateRange | undefined>()

  const [movements, setMovements] = useState<Movement[]>([])
  const [truncated, setTruncated] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    async function load(path: string, apply: (data: { items?: unknown[] }) => void) {
      try {
        const res = await fetchAuth(path)
        if (!res.ok) return
        const data = (await res.json()) as { items?: unknown[] }
        if (!cancelled) apply(data)
      } catch {
        // Leave the selector empty.
      }
    }
    void load("/api/v1/products", (d) => setProducts((d.items ?? []) as Product[]))
    void load("/api/v1/categories", (d) => setCategories((d.items ?? []) as CategoryLike[]))
    void load("/api/v1/events", (d) => setEvents((d.items ?? []) as EventDay[]))
    return () => {
      cancelled = true
    }
  }, [fetchAuth])

  const scope = mode === "product" ? "product" : categoryScope(categories, categoryId)
  const scopeId = mode === "product" ? productId : categoryId
  const params = useMemo(() => (range ? scopeParams(scope, scopeId, range) : null), [scope, scopeId, range])

  const loadMovements = useCallback(
    async (query: URLSearchParams) => {
      setLoading(true)
      setError(null)
      try {
        const search = new URLSearchParams(query)
        search.set("limit", String(PREVIEW_LIMIT))
        const res = await fetchAuth(`/api/v1/inventory/movements?${search}`)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = (await res.json()) as { items?: Movement[]; truncated?: boolean }
        setMovements(data.items ?? [])
        setTruncated(Boolean(data.truncated))
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : "Fehler beim Laden")
        setMovements([])
        setTruncated(false)
      } finally {
        setLoading(false)
      }
    },
    [fetchAuth]
  )

  useEffect(() => {
    if (!params) {
      setMovements([])
      setTruncated(false)
      return
    }
    void loadMovements(params)
  }, [params, loadMovements])

  function selectEventDay(value: string) {
    const day = events.find((e) => eventDayValue(e) === value)
    if (!day) return
    const picked = new Date(day.year, day.month - 1, day.day)
    setRange({ from: picked, to: picked })
  }

  // Not fetch: apiRequest always parses JSON, and window.open carries the session cookie.
  function downloadExport() {
    if (!params) return
    window.open(`/api/v1/inventory/export.xlsx?${params}`, "_blank")
  }

  const showVariant = movements.some((m) => m.variant)

  return (
    <div className="min-w-0 space-y-6">
      <h1 className="text-xl font-semibold">Inventarverlauf</h1>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-3">
          <Tabs value={mode} onValueChange={(v) => setMode(v as ScopeMode)}>
            <TabsList>
              <TabsTrigger value="category">Nach Kategorie</TabsTrigger>
              <TabsTrigger value="product">Nach Produkt</TabsTrigger>
            </TabsList>
          </Tabs>

          {mode === "category" ? (
            <CategorySelect
              categories={categories}
              categoryId={categoryId}
              onChange={setCategoryId}
              idPrefix="inventory"
            />
          ) : (
            <div className="space-y-1">
              <Label htmlFor="inventory-product">Produkt</Label>
              <Select value={productId || undefined} onValueChange={setProductId}>
                <SelectTrigger id="inventory-product" className="h-10">
                  <SelectValue placeholder="Produkt wählen" />
                </SelectTrigger>
                <SelectContent>
                  {products.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>

        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="inventory-range">Zeitraum</Label>
            <DateRangePicker id="inventory-range" value={range} onChange={setRange} />
          </div>

          {events.length > 0 && (
            <div className="space-y-1">
              <Label htmlFor="inventory-event">Verkaufstag übernehmen</Label>
              <Select onValueChange={selectEventDay}>
                <SelectTrigger id="inventory-event" className="h-10">
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

      {!params && !loading && <p className="text-muted-foreground text-sm">Wähle einen Bereich und einen Zeitraum.</p>}

      {params && movements.length === 0 && !loading && !error && (
        <p className="text-muted-foreground text-sm">Keine Bewegungen in diesem Zeitraum.</p>
      )}

      {movements.length > 0 && (
        <>
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Datum</TableHead>
                  <TableHead>Produkt</TableHead>
                  {showVariant && <TableHead>Variante</TableHead>}
                  <TableHead>Delta</TableHead>
                  <TableHead>Grund</TableHead>
                  <TableHead>Bestellung</TableHead>
                  <TableHead>Erfasst von</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {movements.map((m, i) => (
                  <TableRow key={`${m.at}-${m.product}-${i}`}>
                    <TableCell className="text-muted-foreground whitespace-nowrap">
                      {new Date(m.at).toLocaleString("de-CH")}
                    </TableCell>
                    <TableCell>
                      {m.product}
                      {m.subcategory && <span className="text-muted-foreground block text-xs">{m.subcategory}</span>}
                    </TableCell>
                    {showVariant && (
                      <TableCell>{m.variant || <span className="text-muted-foreground">–</span>}</TableCell>
                    )}
                    <TableCell>
                      <span className={m.delta >= 0 ? "font-medium text-green-700" : "font-medium text-red-600"}>
                        {m.delta >= 0 ? `+${m.delta}` : m.delta}
                      </span>
                    </TableCell>
                    <TableCell>{m.reason}</TableCell>
                    <TableCell>
                      {m.orderId ? (
                        <Link
                          href={`/admin/orders/${encodeURIComponent(m.orderId)}`}
                          className="text-xs underline underline-offset-2"
                        >
                          {m.orderId}
                        </Link>
                      ) : (
                        <span className="text-muted-foreground">–</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {m.createdById ? (
                        <Link
                          href={`/admin/users/${encodeURIComponent(m.createdById)}`}
                          className="text-xs underline underline-offset-2"
                        >
                          {m.createdBy || `${m.createdById.slice(0, 8)}…`}
                        </Link>
                      ) : (
                        <span className="text-muted-foreground">–</span>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>

          {truncated && (
            <p className="text-muted-foreground text-sm">
              Nur die ersten {PREVIEW_LIMIT} Bewegungen werden angezeigt. Der Excel-Export enthält alle.
            </p>
          )}
        </>
      )}

      {loading && <p className="text-muted-foreground text-sm">Lade…</p>}
    </div>
  )
}
