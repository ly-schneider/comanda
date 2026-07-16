"use client"

import { Boxes, Check, Loader2, X } from "lucide-react"
import { useCallback, useEffect, useMemo, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

type Product = { id: string; name: string }

function sameSet(a: string[], b: Set<string>): boolean {
  if (a.length !== b.size) return false
  return a.every((id) => b.has(id))
}

/**
 * Station product picker. Uses a plain bearer fetch (not the device-fetch
 * wrapper) so a 403 when the flag is turned off never clears the token and
 * forces re-pairing.
 */
export function StationProductManager({
  token,
  assignedIds,
  onSaved,
}: {
  token: string
  assignedIds: string[]
  onSaved: (products: { productId: string; name?: string }[]) => void
}) {
  const [open, setOpen] = useState(false)
  const [catalog, setCatalog] = useState<Product[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(() => new Set(assignedIds))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [justSaved, setJustSaved] = useState(false)

  useEffect(() => {
    if (!open) return
    setSelected(new Set(assignedIds))
    setError(null)
    setJustSaved(false)
    let cancelled = false
    setLoading(true)
    ;(async () => {
      try {
        const res = await fetch(`/api/v1/products`, { headers: { Authorization: `Bearer ${token}` } })
        if (!res.ok) throw new Error(`Fehler ${res.status}`)
        const data = (await res.json()) as { items?: Product[] }
        if (!cancelled) setCatalog((data.items ?? []).map((p) => ({ id: p.id, name: p.name })))
      } catch {
        if (!cancelled) setError("Produkte konnten nicht geladen werden.")
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [open, assignedIds, token])

  const nameById = useMemo(() => new Map(catalog.map((p) => [p.id, p.name])), [catalog])
  const assignedProducts = useMemo(
    () =>
      Array.from(selected)
        .map((id) => ({ id, name: nameById.get(id) ?? id }))
        .sort((a, b) => a.name.localeCompare(b.name)),
    [selected, nameById]
  )
  const unassigned = useMemo(
    () => catalog.filter((p) => !selected.has(p.id)).sort((a, b) => a.name.localeCompare(b.name)),
    [catalog, selected]
  )
  const dirty = !sameSet(assignedIds, selected)

  const add = useCallback((id: string) => {
    setJustSaved(false)
    setSelected((prev) => new Set(prev).add(id))
  }, [])
  const remove = useCallback((id: string) => {
    setJustSaved(false)
    setSelected((prev) => {
      const next = new Set(prev)
      next.delete(id)
      return next
    })
  }, [])

  const save = useCallback(async () => {
    setSaving(true)
    setError(null)
    setJustSaved(false)
    try {
      const res = await fetch(`/api/v1/stations/me/products`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ productIds: Array.from(selected) }),
      })
      if (res.status === 403) throw new Error("Selbstverwaltung ist nicht mehr aktiviert.")
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { detail?: string; message?: string }
        throw new Error(j.detail || j.message || `Fehler ${res.status}`)
      }
      const data = (await res.json().catch(() => ({}))) as { items?: { productId: string; name?: string }[] }
      const saved = data.items ?? Array.from(selected).map((id) => ({ productId: id, name: nameById.get(id) }))
      onSaved(saved)
      setJustSaved(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }, [selected, token, onSaved])

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-label="Produkte verwalten"
          className="gap-1.5 px-2 sm:px-3"
        >
          <Boxes className="size-4" aria-hidden />
          <span className="hidden sm:inline">Produkte</span>
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Produkte verwalten</DialogTitle>
          <DialogDescription>Wähle, welche Produkte diese Station einlöst.</DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center justify-center py-10">
            <Loader2 className="text-muted-foreground size-5 animate-spin" aria-hidden />
          </div>
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap gap-1.5">
              {assignedProducts.map((p) => (
                <Badge key={p.id} variant="secondary" className="gap-1 pr-1">
                  {p.name}
                  <button
                    type="button"
                    className="hover:bg-destructive/20 rounded p-0.5"
                    onClick={() => remove(p.id)}
                    aria-label={`Produkt ${p.name} entfernen`}
                  >
                    <X className="size-3" />
                  </button>
                </Badge>
              ))}
              {assignedProducts.length === 0 && (
                <span className="text-muted-foreground text-sm">Keine Produkte zugewiesen.</span>
              )}
            </div>

            {unassigned.length > 0 && (
              <Select value="" onValueChange={add}>
                <SelectTrigger className="h-9 text-sm">
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
        )}

        {error && (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        )}

        {justSaved && !error && (
          <p role="status" className="text-success flex items-center gap-1.5 text-sm">
            <Check className="size-4 shrink-0" aria-hidden />
            Gespeichert – die Station scannt jetzt diese Produkte.
          </p>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={saving}>
            {dirty ? "Abbrechen" : "Fertig"}
          </Button>
          <Button type="button" onClick={save} disabled={saving || loading || !dirty}>
            {saving ? "Speichern…" : "Speichern"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
