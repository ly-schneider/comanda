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
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "@/contexts/auth-context"
import { apiRequest, isApiError } from "@/lib/api"
import type { CategoryLike } from "@/lib/categories"
import type { CategoryDTO } from "@/types/category"
import { type ArtistVoucher, createVoucher, deleteVoucher, listVouchers, revokeVoucher, voucherPrintUrl } from "./api"
import { CategoryMultiSelect } from "./category-multi-select"
import { fmtDateTime } from "./format"
import { StatusBadge } from "./status"

export default function ArtistVoucherAdmin() {
  const { getToken } = useAuth()
  const token = getToken() ?? ""
  const [categories, setCategories] = useState<CategoryLike[]>([])
  const [vouchers, setVouchers] = useState<ArtistVoucher[]>([])
  const [error, setError] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [label, setLabel] = useState("")
  const [validFrom, setValidFrom] = useState("")
  const [validUntil, setValidUntil] = useState("")
  const [categoryIds, setCategoryIds] = useState<string[]>([])
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const [revokeTarget, setRevokeTarget] = useState<ArtistVoucher | null>(null)
  const [revoking, setRevoking] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState<ArtistVoucher | null>(null)
  const [deleting, setDeleting] = useState(false)

  const [printTarget, setPrintTarget] = useState<ArtistVoucher | null>(null)
  const [printCount, setPrintCount] = useState<number | "">(5)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    ;(async () => {
      try {
        const [cats, list] = await Promise.all([
          apiRequest<{ items: CategoryDTO[] }>("/v1/categories", { headers: { Authorization: `Bearer ${token}` } }),
          listVouchers(token),
        ])
        if (cancelled) return
        setCategories(cats.items ?? [])
        setVouchers(list)
      } catch {
        if (!cancelled) setError("Gutscheine konnten nicht geladen werden.")
      }
    })()
    return () => {
      cancelled = true
    }
  }, [token])

  function openCreate() {
    setLabel("")
    setValidFrom("")
    setValidUntil("")
    setCategoryIds([])
    setFormError(null)
    setCreateOpen(true)
  }

  async function submitCreate() {
    if (!label.trim()) {
      setFormError("Bitte gib eine Bezeichnung ein.")
      return
    }
    if (!validFrom || !validUntil) {
      setFormError("Bitte gib den Gültigkeitszeitraum an.")
      return
    }
    if (new Date(validUntil) <= new Date(validFrom)) {
      setFormError("Das Enddatum muss nach dem Startdatum liegen.")
      return
    }
    if (categoryIds.length === 0) {
      setFormError("Bitte wähle mindestens eine Kategorie.")
      return
    }
    setSaving(true)
    setFormError(null)
    try {
      const created = await createVoucher(token, {
        label: label.trim(),
        validFrom: new Date(validFrom).toISOString(),
        validUntil: new Date(validUntil).toISOString(),
        categoryIds,
      })
      setVouchers((prev) => [created, ...prev])
      setCreateOpen(false)
    } catch {
      setFormError("Gutschein konnte nicht erstellt werden.")
    } finally {
      setSaving(false)
    }
  }

  async function confirmRevoke() {
    if (!revokeTarget) return
    setRevoking(true)
    try {
      const updated = await revokeVoucher(token, revokeTarget.id)
      setVouchers((prev) => prev.map((v) => (v.id === updated.id ? updated : v)))
      setRevokeTarget(null)
    } catch {
      setError("Gutschein konnte nicht widerrufen werden.")
    } finally {
      setRevoking(false)
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    const target = deleteTarget
    setDeleting(true)
    try {
      await deleteVoucher(token, target.id)
      setVouchers((prev) => prev.filter((v) => v.id !== target.id))
      setDeleteTarget(null)
    } catch (e) {
      setDeleteTarget(null)
      setError(
        isApiError(e) && e.status === 409
          ? "Nur abgelaufene Gutscheine können gelöscht werden."
          : "Gutschein konnte nicht gelöscht werden."
      )
    } finally {
      setDeleting(false)
    }
  }

  function openPrint(v: ArtistVoucher) {
    setPrintCount(5)
    setPrintTarget(v)
  }

  function submitPrint() {
    if (!printTarget) return
    const count = Math.max(1, Math.min(500, printCount === "" ? 5 : printCount))
    window.open(voucherPrintUrl(printTarget.id, count), "_blank")
    setPrintTarget(null)
  }

  const now = Date.now()

  return (
    <Card className="rounded-2xl">
      <CardHeader className="flex flex-row items-start justify-between gap-4">
        <div>
          <CardTitle>Artist-Pässe</CardTitle>
          <p className="text-muted-foreground text-sm">
            Erstelle Gutscheine für Artists und Gäste. Sie werden als A4-Bogen gedruckt und an der Station offline
            geprüft.
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button onClick={openCreate}>Neuer Gutschein</Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}

        {vouchers.length === 0 ? (
          <p className="text-muted-foreground text-sm">Noch keine Gutscheine erstellt.</p>
        ) : (
          <ul className="divide-y rounded-xl border">
            {vouchers.map((v) => (
              <li key={v.id} className="flex items-start justify-between gap-4 p-3">
                <div className="min-w-0 space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium">{v.label}</span>
                    <StatusBadge status={v.status} />
                  </div>
                  <p className="text-muted-foreground text-sm">
                    Gültig für: {v.categories.map((c) => c.name).join(", ") || "—"}
                  </p>
                  <p className="text-muted-foreground text-xs">
                    {fmtDateTime(v.validFrom)} – {fmtDateTime(v.validUntil)}
                  </p>
                </div>
                <div className="flex shrink-0 gap-2">
                  {(v.status === "active" || v.status === "scheduled") && (
                    <Button variant="outline" size="sm" onClick={() => openPrint(v)}>
                      Drucken
                    </Button>
                  )}
                  {(v.status === "active" || v.status === "scheduled") && (
                    <Button variant="outline" size="sm" onClick={() => setRevokeTarget(v)}>
                      Widerrufen
                    </Button>
                  )}
                  {new Date(v.validUntil).getTime() <= now && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      onClick={() => setDeleteTarget(v)}
                    >
                      Löschen
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Neuer Gutschein</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            {formError && (
              <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
                {formError}
              </div>
            )}
            <div className="space-y-1">
              <Label htmlFor="voucher-label">Bezeichnung</Label>
              <Input
                id="voucher-label"
                value={label}
                maxLength={80}
                placeholder="z. B. Live Band"
                onChange={(e) => setLabel(e.target.value)}
              />
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="voucher-from">Gültig ab</Label>
                <Input
                  id="voucher-from"
                  type="datetime-local"
                  value={validFrom}
                  onChange={(e) => setValidFrom(e.target.value)}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="voucher-until">Gültig bis</Label>
                <Input
                  id="voucher-until"
                  type="datetime-local"
                  value={validUntil}
                  onChange={(e) => setValidUntil(e.target.value)}
                />
              </div>
            </div>
            <CategoryMultiSelect categories={categories} selectedIds={categoryIds} onChange={setCategoryIds} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)} disabled={saving}>
              Abbrechen
            </Button>
            <Button onClick={submitCreate} disabled={saving}>
              {saving ? "Erstellen …" : "Erstellen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={revokeTarget !== null} onOpenChange={(open) => !open && setRevokeTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Gutschein widerrufen?</AlertDialogTitle>
            <AlertDialogDescription>
              {revokeTarget && (
                <>
                  Offline-Stationen akzeptieren diesen Gutschein möglicherweise weiterhin, bis sie sich wieder verbinden
                  oder er am {fmtDateTime(revokeTarget.validUntil)} abläuft. Diese Aktion kann nicht rückgängig gemacht
                  werden.
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

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Gutschein löschen?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget && (
                <>
                  «{deleteTarget.label}» ist abgelaufen und wird dauerhaft aus der Liste entfernt. Diese Aktion kann
                  nicht rückgängig gemacht werden.
                </>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Abbrechen</AlertDialogCancel>
            <AlertDialogAction onClick={confirmDelete} disabled={deleting}>
              {deleting ? "Löschen …" : "Löschen"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={printTarget !== null} onOpenChange={(open) => !open && setPrintTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>«{printTarget?.label}» drucken</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <p className="text-muted-foreground text-sm">
              Druckt den QR-Code im Badge-Format (7,6 × 12,7 cm) zum Ausschneiden und Aufkleben – so oft, wie die
              Artists mit ihrer Crew unterwegs sind.
            </p>
            <div className="space-y-1">
              <Label htmlFor="voucher-count">Anzahl</Label>
              <Input
                id="voucher-count"
                type="number"
                min={1}
                max={500}
                className="max-w-24 rounded-xl"
                value={printCount}
                onChange={(e) => setPrintCount(e.target.value === "" ? "" : parseInt(e.target.value, 10))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPrintTarget(null)}>
              Abbrechen
            </Button>
            <Button onClick={submitPrint} disabled={printCount === "" || printCount < 1}>
              Drucken
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
