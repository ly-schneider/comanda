"use client"
import Link from "next/link"
import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { readErrorMessage } from "@/lib/http"
import {
  REFUND_ACTOR_LABELS,
  REFUND_KIND_LABELS,
  REFUND_METHOD_LABELS,
  REFUND_STATUS_COLORS,
  REFUND_STATUS_LABELS,
} from "@/lib/refund-format"
import { formatChf } from "@/lib/utils"
import type { Refund, RefundList } from "@/types/refund"

const PAGE_SIZE = 50

export default function AdminRefundsPage() {
  const fetchAuth = useAuthorizedFetch()
  const [items, setItems] = useState<Refund[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [hasMore, setHasMore] = useState(false)

  const loadPage = useCallback(
    async (offset: number) => {
      setLoading(true)
      setError(null)
      try {
        const res = await fetchAuth(`/api/v1/refunds?limit=${PAGE_SIZE}&offset=${offset}`)
        if (!res.ok) throw new Error(await readErrorMessage(res))
        const data = (await res.json()) as RefundList
        const page = data.items || []
        setItems((prev) => (offset === 0 ? page : [...prev, ...page]))
        setHasMore(page.length === PAGE_SIZE)
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : "Rückerstattungen laden fehlgeschlagen")
      } finally {
        setLoading(false)
      }
    },
    [fetchAuth]
  )

  useEffect(() => {
    void loadPage(0)
  }, [loadPage])

  return (
    <div className="min-w-0 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Rückerstattungen</h1>
      </div>

      {error && <div className="text-sm text-red-600">{error}</div>}

      {items.length === 0 && !loading ? (
        <p className="text-muted-foreground text-sm">Noch keine Rückerstattungen.</p>
      ) : (
        <TableContainer>
          <div
            className="overflow-x-auto overscroll-x-contain"
            role="region"
            aria-label="Rückerstattungen – horizontal scrollen für weitere Spalten"
            tabIndex={0}
          >
            <Table className="whitespace-nowrap">
              <TableHeader className="bg-card sticky top-0">
                <TableRow>
                  <TableHead className="whitespace-nowrap">Datum</TableHead>
                  <TableHead className="whitespace-nowrap">Bestellung</TableHead>
                  <TableHead className="whitespace-nowrap">Betrag</TableHead>
                  <TableHead className="whitespace-nowrap">Art</TableHead>
                  <TableHead className="whitespace-nowrap">Methode</TableHead>
                  <TableHead className="whitespace-nowrap">Status</TableHead>
                  <TableHead className="whitespace-nowrap">Akteur</TableHead>
                  <TableHead className="whitespace-nowrap">Grund</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((r) => (
                  <TableRow key={r.id} className="even:bg-card odd:bg-muted/40">
                    <TableCell className="whitespace-nowrap">{new Date(r.createdAt).toLocaleString("de-CH")}</TableCell>
                    <TableCell>
                      <Link
                        href={`/admin/orders/${encodeURIComponent(r.orderId)}`}
                        className="text-xs underline underline-offset-2"
                      >
                        {r.orderId}
                      </Link>
                    </TableCell>
                    <TableCell className="font-medium">{formatChf(r.amountCents)}</TableCell>
                    <TableCell>{REFUND_KIND_LABELS[r.kind] ?? r.kind}</TableCell>
                    <TableCell>{REFUND_METHOD_LABELS[r.method] ?? r.method}</TableCell>
                    <TableCell>
                      <span
                        className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${REFUND_STATUS_COLORS[r.status] || "bg-gray-100 text-gray-800"}`}
                      >
                        {REFUND_STATUS_LABELS[r.status] ?? r.status}
                      </span>
                    </TableCell>
                    <TableCell className="text-xs" title={r.actorId}>
                      {REFUND_ACTOR_LABELS[r.actorType] ?? r.actorType}
                    </TableCell>
                    <TableCell className="text-muted-foreground max-w-[24rem] truncate" title={r.reason ?? undefined}>
                      {r.reason || "–"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </TableContainer>
      )}

      {hasMore && (
        <div className="flex justify-center">
          <Button variant="outline" disabled={loading} onClick={() => void loadPage(items.length)}>
            {loading ? "Lädt…" : "Mehr laden"}
          </Button>
        </div>
      )}
    </div>
  )
}
