"use client"

import { useEffect, useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { apiRequest } from "@/lib/api"
import type { SlotComponentProps } from "@/plugins/slots"
import type { Product } from "@/types"
import { getMembershipSettings, updateMembershipSettings } from "./api"
import { DEFAULT_LABEL } from "./label"

/**
 * What members get for free. The member system decides it — this screen only
 * offers this tenant's catalog and passes a change on, the same way the budget
 * and the period live over there. A tick saved here is a tick saved in the
 * member system, and a tick made there shows up here.
 */
export default function MembershipAdminSettings({ ctx }: SlotComponentProps<"admin.settings.section">) {
  const { token } = ctx
  const [products, setProducts] = useState<Product[]>([])
  const [freeProductIds, setFreeProductIds] = useState<string[]>([])
  const [label, setLabel] = useState(DEFAULT_LABEL)
  const [synced, setSynced] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    ;(async () => {
      try {
        const [settings, productList] = await Promise.all([
          getMembershipSettings(token),
          apiRequest<{ items: Product[] }>("/v1/products?type=simple&active=true", {
            headers: { Authorization: `Bearer ${token}` },
          }),
        ])
        setFreeProductIds(settings.freeProductIds ?? [])
        setLabel(settings.label || DEFAULT_LABEL)
        setSynced(settings.synced !== false)
        setProducts(productList.items ?? [])
      } catch (e) {
        setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
      }
    })()
  }, [token])

  async function save(ids: string[]) {
    setSaving(true)
    setError(null)
    const before = freeProductIds
    setFreeProductIds(ids)
    try {
      const updated = await updateMembershipSettings(token, ids)
      // What the member system answers wins: it may have refused, or someone
      // may have changed the selection there in the meantime.
      setFreeProductIds(updated.freeProductIds ?? [])
      setLabel(updated.label || DEFAULT_LABEL)
      setSynced(updated.synced !== false)
    } catch (e) {
      setFreeProductIds(before)
      setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card className="rounded-2xl">
      <CardHeader>
        <CardTitle>Mitgliederkarten</CardTitle>
        <p className="text-muted-foreground text-sm">
          Wähle, was Mitglieder gratis bekommen. Gespeichert wird es in eurer Mitgliederverwaltung – dort steht auch, wie viel jemandem zusteht und
          wie das Angebot heisst.
        </p>
      </CardHeader>
      <CardContent className="max-w-lg space-y-4">
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}
        {!synced && (
          <div role="alert" className="bg-muted rounded px-3 py-2 text-sm">
            Eure Mitgliederverwaltung ist gerade nicht erreichbar. Unten steht der letzte bekannte Stand; Änderungen lassen sich erst wieder
            speichern, wenn sie antwortet.
          </div>
        )}

        <div className="space-y-2">
          <p className="text-sm font-medium">Angebot</p>
          <p className="text-muted-foreground text-sm">
            «{label}» – so heisst es an der Kasse auf der Kachel und über der Seite im Shop.
          </p>
        </div>

        <div className="space-y-2">
          <p className="text-sm font-medium">Gratis-Produkte</p>
          <div className="max-h-48 space-y-1 overflow-y-auto rounded-xl border p-2">
            {products.map((p) => (
              <label key={p.id} className="hover:bg-muted flex cursor-pointer items-center gap-2 rounded-lg p-2">
                <Checkbox
                  checked={freeProductIds.includes(p.id)}
                  onCheckedChange={(checked) => save(checked ? [...freeProductIds, p.id] : freeProductIds.filter((id) => id !== p.id))}
                  disabled={saving || !synced}
                />
                <span>{p.name}</span>
              </label>
            ))}
          </div>
          {freeProductIds.length === 0 && <p className="text-muted-foreground text-sm">Keine Produkte ausgewählt</p>}
        </div>
      </CardContent>
    </Card>
  )
}
