"use client"

import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { type ArtistVoucherBatchDetail, type ArtistVoucherBatchVoucher, getBatch } from "./api"
import { entitlementText, fmtDateTime, outstandingStockText } from "./format"
import { StatusBadge } from "./status"

function redemptionCell(v: ArtistVoucherBatchVoucher): string {
  if (v.redeemedAt) return fmtDateTime(v.redeemedAt)
  if (v.revokedAt) return `Widerrufen am ${fmtDateTime(v.revokedAt)}`
  return "—"
}

export function BatchDetailDialog({
  token,
  batchId,
  onClose,
}: {
  token: string
  batchId: string
  onClose: () => void
}) {
  const [detail, setDetail] = useState<ArtistVoucherBatchDetail | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const loaded = await getBatch(token, batchId)
        if (!cancelled) setDetail(loaded)
      } catch {
        if (!cancelled) setError("Die Gutschein-Serie konnte nicht geladen werden.")
      }
    })()
    return () => {
      cancelled = true
    }
  }, [token, batchId])

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{detail ? `«${detail.batch.label}»` : "Gutschein-Serie"}</DialogTitle>
        </DialogHeader>
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}
        {!detail && !error && <p className="text-muted-foreground text-sm">Wird geladen …</p>}
        {detail && (
          <div className="space-y-3">
            <div className="space-y-1">
              <p className="font-semibold">{outstandingStockText(detail.batch)}</p>
              <p className="text-muted-foreground text-sm">{entitlementText(detail.batch.products)}</p>
              <p className="text-muted-foreground text-xs">
                {fmtDateTime(detail.batch.validFrom)} – {fmtDateTime(detail.batch.validUntil)}
              </p>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-12">Nr.</TableHead>
                  <TableHead>Gutschein</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Eingelöst am</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {detail.vouchers.map((v, index) => (
                  <TableRow key={v.id}>
                    <TableCell className="text-muted-foreground">{index + 1}</TableCell>
                    <TableCell className="font-mono text-xs">{v.id}</TableCell>
                    <TableCell>
                      <StatusBadge status={v.status} />
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{redemptionCell(v)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
