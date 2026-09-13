"use client"

import { Plus, Trash2 } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { isApiError } from "@/lib/api"
import type { ProductSummaryDTO } from "@/types/product"
import { type ArtistVoucherBatch, createBatch } from "./api"

const MAX_COUNT = 500

type ProductRow = { productId: string; quantity: number | ""; choiceGroup: number }

export function BatchCreateDialog({
  token,
  products,
  onClose,
  onCreated,
}: {
  token: string
  products: ProductSummaryDTO[]
  onClose: () => void
  onCreated: (batch: ArtistVoucherBatch) => void
}) {
  const [label, setLabel] = useState("")
  const [validFrom, setValidFrom] = useState("")
  const [validUntil, setValidUntil] = useState("")
  const [rows, setRows] = useState<ProductRow[]>([{ productId: "", quantity: 1, choiceGroup: 0 }])
  const [count, setCount] = useState<number | "">(50)
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const eligible = products.filter((p) => p.type === "simple" && (p.variants?.length ?? 0) === 0)
  const usedIds = new Set(rows.map((r) => r.productId).filter(Boolean))
  const hasFreeProduct = eligible.some((p) => !usedIds.has(p.id))
  const usedGroups = Array.from(new Set(rows.map((r) => r.choiceGroup).filter((g) => g > 0))).sort((a, b) => a - b)
  const groupOptions = [...usedGroups, (usedGroups.at(-1) ?? 0) + 1]

  function updateRow(index: number, patch: Partial<ProductRow>) {
    setRows((rs) => rs.map((r, i) => (i === index ? { ...r, ...patch } : r)))
  }

  async function submit() {
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
    if (rows.length === 0 || rows.some((r) => !r.productId)) {
      setFormError("Bitte für jede Zeile ein Produkt wählen.")
      return
    }
    const tooSmall = usedGroups.find((g) => rows.filter((r) => r.choiceGroup === g).length < 2)
    if (tooSmall !== undefined) {
      setFormError(`Auswahl ${tooSmall} braucht mindestens zwei Produkte, sonst gibt es nichts zu wählen.`)
      return
    }
    const slips = count === "" ? 0 : count
    if (slips < 1 || slips > MAX_COUNT) {
      setFormError(`Die Anzahl muss zwischen 1 und ${MAX_COUNT} liegen.`)
      return
    }
    setSaving(true)
    setFormError(null)
    try {
      const created = await createBatch(token, {
        label: label.trim(),
        validFrom: new Date(validFrom).toISOString(),
        validUntil: new Date(validUntil).toISOString(),
        products: rows.map((r) => ({
          productId: r.productId,
          quantity: Math.max(1, r.quantity === "" ? 1 : r.quantity),
          choiceGroup: r.choiceGroup,
        })),
        count: slips,
      })
      onCreated(created)
      onClose()
    } catch (e) {
      setFormError(isApiError(e) && e.status === 400 ? e.message : "Die Gutschein-Serie konnte nicht erstellt werden.")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Neue Gutschein-Serie</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          {formError && (
            <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
              {formError}
            </div>
          )}
          <div className="space-y-1">
            <Label htmlFor="batch-label">Bezeichnung</Label>
            <Input
              id="batch-label"
              value={label}
              maxLength={80}
              placeholder="z. B. Verteilaktion Oktober"
              onChange={(e) => setLabel(e.target.value)}
            />
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="batch-from">Gültig ab</Label>
              <Input
                id="batch-from"
                type="datetime-local"
                value={validFrom}
                onChange={(e) => setValidFrom(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="batch-until">Gültig bis</Label>
              <Input
                id="batch-until"
                type="datetime-local"
                value={validUntil}
                onChange={(e) => setValidUntil(e.target.value)}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label>Produkte pro Gutschein</Label>
            {eligible.length === 0 ? (
              <p className="text-muted-foreground text-sm">Keine geeigneten Produkte vorhanden.</p>
            ) : (
              <>
                {rows.map((row, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <Select value={row.productId} onValueChange={(v) => updateRow(index, { productId: v })}>
                      <SelectTrigger className="flex-1" aria-label="Produkt">
                        <SelectValue placeholder="Produkt wählen …" />
                      </SelectTrigger>
                      <SelectContent>
                        {eligible
                          .filter((p) => p.id === row.productId || !usedIds.has(p.id))
                          .map((p) => (
                            <SelectItem key={p.id} value={p.id}>
                              {p.name}
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                    <Input
                      type="number"
                      min={1}
                      className="w-16"
                      aria-label="Menge"
                      value={row.quantity}
                      onChange={(e) =>
                        updateRow(index, {
                          quantity: e.target.value === "" ? "" : parseInt(e.target.value, 10),
                        })
                      }
                    />
                    <Select
                      value={String(row.choiceGroup)}
                      onValueChange={(v) => updateRow(index, { choiceGroup: parseInt(v, 10) })}
                    >
                      <SelectTrigger className="w-36" aria-label="Auswahlgruppe">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="0">Immer dabei</SelectItem>
                        {groupOptions.map((g) => (
                          <SelectItem key={g} value={String(g)}>
                            Auswahl {g}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {rows.length > 1 && (
                      <Button
                        variant="ghost"
                        size="icon"
                        className="text-muted-foreground hover:text-destructive size-9 shrink-0"
                        aria-label="Produkt entfernen"
                        onClick={() => setRows((rs) => rs.filter((_, i) => i !== index))}
                      >
                        <Trash2 className="size-4" aria-hidden />
                      </Button>
                    )}
                  </div>
                ))}
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-muted-foreground h-7 px-1.5 text-xs"
                  disabled={!hasFreeProduct}
                  onClick={() => setRows((rs) => [...rs, { productId: "", quantity: 1, choiceGroup: 0 }])}
                >
                  <Plus className="size-3.5" aria-hidden />
                  Produkt hinzufügen
                </Button>
                <p className="text-muted-foreground text-xs">
                  Produkte mit «Immer dabei» stehen auf jedem Gutschein. Produkte in derselben Auswahl sind
                  Alternativen: Am Stand wird genau eines davon ausgegeben. So gibt es zum Beispiel immer Pommes und
                  dazu wahlweise einen Hamburger oder einen Cheeseburger. Auswählbar sind nur einfache Produkte ohne
                  Varianten.
                </p>
              </>
            )}
          </div>
          <div className="space-y-1">
            <Label htmlFor="batch-count">Anzahl Gutscheine</Label>
            <Input
              id="batch-count"
              type="number"
              min={1}
              max={MAX_COUNT}
              className="max-w-24 rounded-xl"
              value={count}
              onChange={(e) => setCount(e.target.value === "" ? "" : parseInt(e.target.value, 10))}
            />
            <p className="text-muted-foreground text-xs">
              Jeder Gutschein bekommt einen eigenen QR-Code und lässt sich genau einmal einlösen.
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={saving}>
            Abbrechen
          </Button>
          <Button onClick={submit} disabled={saving}>
            {saving ? "Erstellen …" : "Erstellen"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
