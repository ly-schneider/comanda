"use client"

import { useEffect, useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { apiRequest } from "@/lib/api"
import type { SlotComponentProps } from "@/plugins/slots"
import type { Product } from "@/types"
import { getClub100Settings, updateClub100Settings } from "./api"

export default function Club100AdminSettings({ ctx }: SlotComponentProps<"admin.settings.section">) {
  const { token } = ctx
  const [products, setProducts] = useState<Product[]>([])
  const [freeProductIds, setFreeProductIds] = useState<string[]>([])
  const [maxRedemptions, setMaxRedemptions] = useState<number | "">(2)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    ;(async () => {
      try {
        const [settings, productList] = await Promise.all([
          getClub100Settings(token),
          apiRequest<{ items: Product[] }>("/v1/products?type=simple&active=true", {
            headers: { Authorization: `Bearer ${token}` },
          }),
        ])
        setFreeProductIds(settings.freeProductIds ?? [])
        setMaxRedemptions(settings.maxRedemptions ?? 2)
        setProducts(productList.items ?? [])
      } catch (e) {
        setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
      }
    })()
  }, [token])

  async function save(patch: { freeProductIds?: string[]; maxRedemptions?: number }) {
    setSaving(true)
    setError(null)
    try {
      const updated = await updateClub100Settings(token, patch)
      setFreeProductIds(updated.freeProductIds ?? [])
      setMaxRedemptions(updated.maxRedemptions ?? maxRedemptions)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card className="rounded-2xl">
      <CardHeader>
        <CardTitle>100 Club Einstellungen</CardTitle>
        <p className="text-muted-foreground text-sm">Wähle die Produkte, die 100 Club Mitglieder gratis erhalten.</p>
      </CardHeader>
      <CardContent className="max-w-lg space-y-4">
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}
        <div className="space-y-2">
          <Label>Gratis-Produkte</Label>
          <div className="max-h-48 space-y-1 overflow-y-auto rounded-xl border p-2">
            {products.map((p) => (
              <label key={p.id} className="hover:bg-muted flex cursor-pointer items-center gap-2 rounded-lg p-2">
                <Checkbox
                  checked={freeProductIds.includes(p.id)}
                  onCheckedChange={(checked) => {
                    const newIds = checked ? [...freeProductIds, p.id] : freeProductIds.filter((id) => id !== p.id)
                    setFreeProductIds(newIds)
                    save({ freeProductIds: newIds })
                  }}
                  disabled={saving}
                />
                <span>{p.name}</span>
              </label>
            ))}
          </div>
          {freeProductIds.length === 0 && <p className="text-muted-foreground text-sm">Keine Produkte ausgewählt</p>}
        </div>

        <div className="space-y-2">
          <Label htmlFor="club100-max">Max. Einlösungen pro Person</Label>
          <Input
            id="club100-max"
            type="number"
            min={1}
            value={maxRedemptions}
            onChange={(e) => {
              const raw = e.target.value
              setMaxRedemptions(raw === "" ? "" : parseInt(raw, 10))
            }}
            onBlur={() => {
              const value = maxRedemptions === "" || !maxRedemptions ? 1 : maxRedemptions
              if (value !== maxRedemptions) setMaxRedemptions(value)
              save({ maxRedemptions: value })
            }}
            disabled={saving}
            className="max-w-24 rounded-xl"
          />
        </div>
      </CardContent>
    </Card>
  )
}
