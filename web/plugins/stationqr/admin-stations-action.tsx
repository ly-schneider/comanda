"use client"

import { Check, FileText, Loader2, Pencil, Plus, QrCode, RefreshCw, Trash2, X } from "lucide-react"
import QRCode from "qrcode"
import { useCallback, useEffect, useMemo, useState } from "react"
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
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useBranding } from "@/contexts/branding-context"
import type { SlotComponentProps } from "@/plugins/slots"
import {
  createPreset,
  deletePreset,
  listPresets,
  listProducts,
  type Product,
  rotatePreset,
  type StationqrPreset,
  updatePreset,
} from "./api"

// Best-effort: pull the tenant logo (R2, cross-origin) into a data URL for the
// PDF. Returns null if the image can't be loaded or the canvas taints (no CORS).
function loadImageDataUrl(url: string): Promise<{ dataUrl: string; w: number; h: number } | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.crossOrigin = "anonymous"
    img.onload = () => {
      try {
        const canvas = document.createElement("canvas")
        canvas.width = img.naturalWidth
        canvas.height = img.naturalHeight
        const ctx = canvas.getContext("2d")
        if (!ctx) return resolve(null)
        ctx.drawImage(img, 0, 0)
        resolve({ dataUrl: canvas.toDataURL("image/png"), w: img.naturalWidth, h: img.naturalHeight })
      } catch {
        resolve(null)
      }
    }
    img.onerror = () => resolve(null)
    img.src = url
  })
}

async function makePdf(token: string, presetName: string, displayName: string, logoUrl: string | null): Promise<Blob> {
  const [{ jsPDF }, qrPng, logo] = await Promise.all([
    import("jspdf"),
    QRCode.toDataURL(token, { width: 1200, margin: 1 }),
    logoUrl ? loadImageDataUrl(logoUrl) : Promise.resolve(null),
  ])
  const doc = new jsPDF({ unit: "mm", format: "a4" })
  const pageW = doc.internal.pageSize.getWidth()
  const cx = pageW / 2
  let y = 36

  if (logo) {
    const ratio = logo.w / logo.h
    let lw = 22 * ratio
    let lh = 22
    if (lw > 60) {
      lw = 60
      lh = 60 / ratio
    }
    doc.addImage(logo.dataUrl, "PNG", cx - lw / 2, y, lw, lh)
    y += lh + 10
  }

  doc.setFont("helvetica", "bold")
  doc.setFontSize(22)
  doc.text(displayName, cx, y, { align: "center", maxWidth: pageW - 30 })
  y += 11
  doc.setFontSize(15)
  doc.text(presetName, cx, y, { align: "center", maxWidth: pageW - 30 })
  y += 14

  const qrSize = 100
  doc.addImage(qrPng, "PNG", cx - qrSize / 2, y, qrSize, qrSize)
  y += qrSize + 12

  doc.setFont("helvetica", "normal")
  doc.setFontSize(12)
  doc.setTextColor(90)
  doc.text("Auf einem neuen Stationsgerät scannen.", cx, y, { align: "center", maxWidth: pageW - 40 })

  return doc.output("blob")
}

export default function StationqrAdminStationsAction({ ctx }: SlotComponentProps<"admin.stations.actions">) {
  const { token } = ctx
  const branding = useBranding()
  const [presets, setPresets] = useState<StationqrPreset[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [formName, setFormName] = useState("")
  const [formProductIds, setFormProductIds] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    ;(async () => {
      try {
        const [ps, pr] = await Promise.all([listPresets(token), listProducts(token)])
        setPresets(ps)
        setProducts(pr)
      } catch (e) {
        setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
      } finally {
        setLoading(false)
      }
    })()
  }, [token])

  const replacePreset = useCallback((p: StationqrPreset) => {
    setPresets((prev) => prev.map((x) => (x.id === p.id ? p : x)))
  }, [])

  const openCreate = () => {
    setFormName("")
    setFormProductIds([])
    setFormError(null)
    setCreateOpen(true)
  }

  const submitCreate = useCallback(async () => {
    const name = formName.trim()
    if (!name) {
      setFormError("Name erforderlich.")
      return
    }
    setSaving(true)
    setFormError(null)
    try {
      const created = await createPreset(token, { name, productIds: formProductIds })
      setPresets((prev) => [...prev, created])
      setCreateOpen(false)
    } catch (e) {
      setFormError(e instanceof Error ? e.message : "Erstellen fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }, [token, formName, formProductIds])

  const handlers = useMemo(
    () => ({
      onRename: async (id: string, name: string) => replacePreset(await updatePreset(token, id, { name })),
      onToggle: async (id: string, active: boolean) =>
        replacePreset(await updatePreset(token, id, { disabled: !active })),
      onSetProducts: async (id: string, productIds: string[]) =>
        replacePreset(await updatePreset(token, id, { productIds })),
      onRotate: async (id: string) => replacePreset(await rotatePreset(token, id)),
      onDelete: async (id: string) => {
        await deletePreset(token, id)
        setPresets((prev) => prev.filter((p) => p.id !== id))
      },
    }),
    [token, replacePreset]
  )

  return (
    <Card className="mb-6 rounded-2xl">
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>Schnell-Registrierung per QR</CardTitle>
          <p className="text-muted-foreground text-sm">
            Lege je Station einen QR-Code mit passenden Produkten an. Wird er auf einem neuen Gerät gescannt, meldet
            sich die Station an und erhält direkt diese Produkte — beliebig oft, ohne Kopplungscode.
          </p>
        </div>
        <Button onClick={openCreate} className="shrink-0">
          <Plus className="mr-2 size-4" />
          Neuer QR-Code
        </Button>
      </CardHeader>
      <CardContent>
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 mb-4 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}

        {loading ? (
          <Loader2 className="text-muted-foreground size-5 animate-spin" aria-hidden />
        ) : presets.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            Noch keine QR-Codes. Erstelle den ersten und weise ihm die Produkte der Station zu.
          </p>
        ) : (
          <div className="space-y-3">
            {presets.map((p) => (
              <PresetCard
                key={p.id}
                preset={p}
                allProducts={products}
                displayName={branding.displayName}
                logoUrl={branding.logoUrl}
                onError={setError}
                {...handlers}
              />
            ))}
          </div>
        )}
      </CardContent>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Neuer Registrierungs-QR</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4">
            <div className="grid gap-2">
              <Label htmlFor="stationqr-preset-name">Name</Label>
              <Input
                id="stationqr-preset-name"
                value={formName}
                onChange={(e) => setFormName(e.target.value)}
                placeholder="z.B. Burger-Stand"
                maxLength={60}
                autoFocus
              />
            </div>
            <div className="grid gap-2">
              <Label>Produkte (optional)</Label>
              <ProductPicker
                allProducts={products}
                selectedIds={formProductIds}
                onChange={setFormProductIds}
                emptyHint="Ohne Produkte meldet sich die Station nur an."
              />
            </div>
            {formError && (
              <div role="alert" className="bg-destructive/10 text-destructive rounded-xl p-3 text-sm">
                {formError}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)} disabled={saving}>
              Abbrechen
            </Button>
            <Button onClick={submitCreate} disabled={saving}>
              {saving ? <Loader2 className="mr-2 size-4 animate-spin" /> : <QrCode className="mr-2 size-4" />}
              Erstellen
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

function PresetCard({
  preset,
  allProducts,
  displayName,
  logoUrl,
  onRename,
  onToggle,
  onSetProducts,
  onRotate,
  onDelete,
  onError,
}: {
  preset: StationqrPreset
  allProducts: Product[]
  displayName: string
  logoUrl: string | null
  onRename: (id: string, name: string) => Promise<void>
  onToggle: (id: string, active: boolean) => Promise<void>
  onSetProducts: (id: string, productIds: string[]) => Promise<void>
  onRotate: (id: string) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onError: (msg: string) => void
}) {
  const [qrDataUrl, setQrDataUrl] = useState("")
  const [busy, setBusy] = useState(false)
  const [pdfBusy, setPdfBusy] = useState(false)
  const [editing, setEditing] = useState(false)
  const [editName, setEditName] = useState(preset.name)
  const [confirmRotate, setConfirmRotate] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  useEffect(() => {
    QRCode.toDataURL(preset.token, { width: 240, margin: 1 })
      .then(setQrDataUrl)
      .catch(() => setQrDataUrl(""))
  }, [preset.token])

  const run = useCallback(
    async (fn: () => Promise<void>, fallback: string) => {
      setBusy(true)
      try {
        await fn()
      } catch (e) {
        onError(e instanceof Error ? e.message : fallback)
      } finally {
        setBusy(false)
      }
    },
    [onError]
  )

  const openPdf = useCallback(async () => {
    // Open the tab inside the click gesture so the pop-up blocker doesn't swallow
    // it, then point it at the generated PDF once it's ready.
    const win = window.open("", "_blank")
    setPdfBusy(true)
    try {
      const url = URL.createObjectURL(await makePdf(preset.token, preset.name, displayName, logoUrl))
      if (win) win.location.href = url
      else window.open(url, "_blank")
    } catch {
      win?.close()
      onError("PDF konnte nicht erstellt werden.")
    } finally {
      setPdfBusy(false)
    }
  }, [preset.token, preset.name, displayName, logoUrl, onError])

  return (
    <div className={`bg-card border-border rounded-[11px] border p-3 ${preset.disabled ? "opacity-70" : ""}`}>
      <div className="flex flex-col gap-4 sm:flex-row">
        <div className={`shrink-0 self-start rounded-xl border bg-white p-3 ${preset.disabled ? "opacity-40" : ""}`}>
          {qrDataUrl ? (
            // eslint-disable-next-line @next/next/no-img-element -- local data URL, not a remote asset to optimize
            <img src={qrDataUrl} alt={`QR-Code ${preset.name}`} width={150} height={150} />
          ) : (
            <div className="flex size-[150px] items-center justify-center">
              <Loader2 className="text-muted-foreground size-5 animate-spin" aria-hidden />
            </div>
          )}
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-3">
          {editing ? (
            <form
              className="flex items-center gap-1.5"
              onSubmit={(e) => {
                e.preventDefault()
                const trimmed = editName.trim()
                if (trimmed && trimmed !== preset.name)
                  void run(() => onRename(preset.id, trimmed), "Umbenennen fehlgeschlagen")
                setEditing(false)
              }}
            >
              <Input
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                maxLength={60}
                className="h-7 w-48 text-sm font-semibold"
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === "Escape") {
                    setEditName(preset.name)
                    setEditing(false)
                  }
                }}
              />
              <Button type="submit" variant="ghost" size="icon" className="size-7">
                <Check className="size-3.5" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-7"
                onClick={() => {
                  setEditName(preset.name)
                  setEditing(false)
                }}
              >
                <X className="size-3.5" />
              </Button>
            </form>
          ) : (
            <div className="flex items-center gap-1.5">
              <div className="truncate font-semibold">{preset.name}</div>
              <button
                className="text-muted-foreground hover:text-foreground shrink-0"
                onClick={() => {
                  setEditName(preset.name)
                  setEditing(true)
                }}
                aria-label="Umbenennen"
              >
                <Pencil className="size-3.5" />
              </button>
            </div>
          )}

          <ProductPicker
            allProducts={allProducts}
            selectedIds={preset.productIds}
            disabled={busy}
            onChange={(ids) => void run(() => onSetProducts(preset.id, ids), "Produkte speichern fehlgeschlagen")}
            emptyHint="Keine Produkte — meldet die Station nur an."
          />

          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={openPdf} disabled={pdfBusy}>
              {pdfBusy ? <Loader2 className="mr-2 size-4 animate-spin" /> : <FileText className="mr-2 size-4" />}
              PDF öffnen
            </Button>
            <Button variant="outline" size="sm" onClick={() => setConfirmRotate(true)} disabled={busy}>
              <RefreshCw className="mr-2 size-4" />
              Neu generieren
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={() => setConfirmDelete(true)}
              disabled={busy}
            >
              <Trash2 className="mr-2 size-4" />
              Löschen
            </Button>
            <div className="ml-auto flex items-center gap-2">
              <Switch
                id={`stationqr-active-${preset.id}`}
                checked={!preset.disabled}
                disabled={busy}
                onCheckedChange={(active) => void run(() => onToggle(preset.id, active), "Speichern fehlgeschlagen")}
              />
              <Label htmlFor={`stationqr-active-${preset.id}`} className="text-sm">
                Aktiv
              </Label>
            </div>
          </div>
          {preset.disabled && (
            <p className="text-muted-foreground text-xs">
              Deaktiviert — Stationen können sich hierüber nicht anmelden.
            </p>
          )}
        </div>
      </div>

      <AlertDialog open={confirmRotate} onOpenChange={setConfirmRotate}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Neuen Code generieren?</AlertDialogTitle>
            <AlertDialogDescription>
              Der bisherige Ausdruck von &quot;{preset.name}&quot; wird ungültig. Stationen müssen anschliessend den
              neuen Code scannen.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                setConfirmRotate(false)
                void run(() => onRotate(preset.id), "Neu generieren fehlgeschlagen")
              }}
              disabled={busy}
            >
              Neu generieren
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>QR-Code löschen</AlertDialogTitle>
            <AlertDialogDescription>
              &quot;{preset.name}&quot; wird gelöscht und der Ausdruck ungültig. Bereits angemeldete Stationen bleiben
              bestehen.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => {
                setConfirmDelete(false)
                void run(() => onDelete(preset.id), "Löschen fehlgeschlagen")
              }}
              disabled={busy}
            >
              Löschen
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ProductPicker({
  allProducts,
  selectedIds,
  disabled,
  onChange,
  emptyHint,
}: {
  allProducts: Product[]
  selectedIds: string[]
  disabled?: boolean
  onChange: (ids: string[]) => void
  emptyHint: string
}) {
  const selectedSet = useMemo(() => new Set(selectedIds), [selectedIds])
  const selected = useMemo(
    () => selectedIds.map((id) => allProducts.find((p) => p.id === id)).filter((p): p is Product => !!p),
    [selectedIds, allProducts]
  )
  const unassigned = useMemo(() => allProducts.filter((p) => !selectedSet.has(p.id)), [allProducts, selectedSet])

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-1.5">
        {selected.map((p) => (
          <Badge key={p.id} variant="secondary" className="gap-1 pr-1">
            {p.name}
            <button
              className="hover:bg-destructive/20 rounded p-0.5 disabled:opacity-50"
              disabled={disabled}
              onClick={() => onChange(selectedIds.filter((id) => id !== p.id))}
              aria-label={`Produkt ${p.name} entfernen`}
            >
              <X className="size-3" />
            </button>
          </Badge>
        ))}
        {selected.length === 0 && <span className="text-muted-foreground text-sm">{emptyHint}</span>}
      </div>
      {unassigned.length > 0 && (
        <Select value="" disabled={disabled} onValueChange={(id) => onChange([...selectedIds, id])}>
          <SelectTrigger className="h-8 text-xs">
            <SelectValue placeholder="Produkt hinzufügen…" />
          </SelectTrigger>
          <SelectContent>
            {unassigned.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  )
}
