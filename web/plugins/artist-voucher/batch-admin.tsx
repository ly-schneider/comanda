"use client"

import { useEffect, useState } from "react"
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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuth } from "@/contexts/auth-context"
import { apiRequest } from "@/lib/api"
import type { ProductSummaryDTO } from "@/types/product"
import { type ArtistVoucherBatch, batchPrintUrl, listBatches, revokeBatch } from "./api"
import { BatchCreateDialog } from "./batch-create-dialog"
import { BatchDetailDialog } from "./batch-detail-dialog"
import { entitlementText, fmtDateTime, outstandingStockText } from "./format"
import { StatusBadge } from "./status"

export default function VoucherBatchAdmin() {
  const { getToken } = useAuth()
  const token = getToken() ?? ""
  const [batches, setBatches] = useState<ArtistVoucherBatch[]>([])
  const [products, setProducts] = useState<ProductSummaryDTO[]>([])
  const [error, setError] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [detailId, setDetailId] = useState<string | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<ArtistVoucherBatch | null>(null)
  const [revoking, setRevoking] = useState(false)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    ;(async () => {
      try {
        const [list, catalog] = await Promise.all([
          listBatches(token),
          apiRequest<{ items: ProductSummaryDTO[] }>("/v1/products", {
            headers: { Authorization: `Bearer ${token}` },
          }),
        ])
        if (cancelled) return
        setBatches(list)
        setProducts(catalog.items ?? [])
      } catch {
        if (!cancelled) setError("Gutschein-Serien konnten nicht geladen werden.")
      }
    })()
    return () => {
      cancelled = true
    }
  }, [token])

  async function confirmRevoke() {
    if (!revokeTarget) return
    setRevoking(true)
    try {
      const updated = await revokeBatch(token, revokeTarget.id)
      setBatches((prev) => prev.map((b) => (b.id === updated.id ? updated : b)))
      setRevokeTarget(null)
    } catch {
      setError("Die Gutschein-Serie konnte nicht widerrufen werden.")
    } finally {
      setRevoking(false)
    }
  }

  return (
    <Card className="rounded-2xl">
      <CardHeader className="flex flex-row items-start justify-between gap-4">
        <div>
          <CardTitle>Gutschein-Serien</CardTitle>
          <p className="text-muted-foreground text-sm">
            Einzeln nummerierte Gutscheine zum Verteilen – jeder Slip wird genau einmal eingelöst. Der Lagerbestand
            sinkt erst beim Einlösen am Stand, nicht beim Drucken.
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button onClick={() => setCreateOpen(true)}>Neue Serie</Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}

        {batches.length === 0 ? (
          <p className="text-muted-foreground text-sm">Noch keine Serien ausgegeben.</p>
        ) : (
          <ul className="divide-y rounded-xl border">
            {batches.map((b) => {
              const open = b.status === "active" || b.status === "scheduled"
              return (
                <li key={b.id} className="flex flex-wrap items-start justify-between gap-4 p-3">
                  <div className="min-w-0 space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium">{b.label}</span>
                      <StatusBadge status={b.status} />
                    </div>
                    <p className="text-base font-semibold">{outstandingStockText(b)}</p>
                    <p className="text-muted-foreground text-sm">{entitlementText(b.products)}</p>
                    <p className="text-muted-foreground text-xs">
                      {fmtDateTime(b.validFrom)} – {fmtDateTime(b.validUntil)}
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {b.issued} ausgegeben · {b.redeemed} eingelöst · {b.revoked} widerrufen
                    </p>
                  </div>
                  <div className="flex shrink-0 gap-2">
                    <Button variant="outline" size="sm" onClick={() => setDetailId(b.id)}>
                      Details
                    </Button>
                    {open && (
                      <Button variant="outline" size="sm" onClick={() => window.open(batchPrintUrl(b.id), "_blank")}>
                        Drucken
                      </Button>
                    )}
                    {open && b.outstanding > 0 && (
                      <Button variant="outline" size="sm" onClick={() => setRevokeTarget(b)}>
                        Widerrufen
                      </Button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>

      {createOpen && (
        <BatchCreateDialog
          token={token}
          products={products}
          onClose={() => setCreateOpen(false)}
          onCreated={(batch) => setBatches((prev) => [batch, ...prev])}
        />
      )}

      {detailId && <BatchDetailDialog token={token} batchId={detailId} onClose={() => setDetailId(null)} />}

      <AlertDialog open={revokeTarget !== null} onOpenChange={(open) => !open && setRevokeTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Offene Gutscheine widerrufen?</AlertDialogTitle>
            <AlertDialogDescription>
              {revokeTarget && (
                <>
                  Die {revokeTarget.outstanding} noch offenen Slips von «{revokeTarget.label}» werden ungültig, bereits
                  eingelöste bleiben unverändert. Offline-Stationen akzeptieren sie möglicherweise weiterhin, bis sie
                  sich wieder verbinden oder die Serie am {fmtDateTime(revokeTarget.validUntil)} abläuft. Diese Aktion
                  kann nicht rückgängig gemacht werden.
                </>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={revoking}>Abbrechen</AlertDialogCancel>
            <AlertDialogAction onClick={confirmRevoke} disabled={revoking}>
              {revoking ? "Widerrufen …" : "Widerrufen"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}
