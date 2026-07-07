"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { PluginSlot } from "@/components/plugins/plugin-slot"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Switch } from "@/components/ui/switch"
import { useAuth } from "@/contexts/auth-context"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type PosFulfillmentMode = "QR_CODE" | "JETON" | "HYBRID"

type SettingsResponse = {
  systemEnabled?: boolean
  posMode?: PosFulfillmentMode
  missingJetons?: number
}

export default function AdminSettingsPage() {
  const fetchAuth = useAuthorizedFetch()
  const { getToken } = useAuth()
  const token = getToken() ?? ""

  const [systemEnabled, setSystemEnabled] = useState(true)
  const [posMode, setPosMode] = useState<PosFulfillmentMode>("QR_CODE")
  const [missingJetons, setMissingJetons] = useState(0)

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    ;(async () => {
      const settingsRes = await fetchAuth(`/api/v1/settings`)
      if (settingsRes.ok) {
        const s = (await settingsRes.json()) as SettingsResponse
        setSystemEnabled(s.systemEnabled ?? true)
        setPosMode(s.posMode ?? "QR_CODE")
        setMissingJetons(s.missingJetons ?? 0)
      }
    })()
  }, [fetchAuth])

  async function updateSettings(updates: Partial<SettingsResponse>) {
    setSaving(true)
    setError(null)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`/api/v1/settings`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify(updates),
      })
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as {
          error?: string
          missing?: number
          message?: string
          detail?: string
        }
        if (j.error === "missing_jetons") {
          setMissingJetons(j.missing ?? missingJetons)
          throw new Error(`Jeton-Modus kann nicht aktiviert werden. ${j.missing} aktive Produkte haben keinen Jeton.`)
        }
        throw new Error(j.detail || j.message || `Error ${res.status}`)
      }
      const updated = (await res.json()) as SettingsResponse
      setSystemEnabled(updated.systemEnabled ?? systemEnabled)
      setPosMode(updated.posMode ?? posMode)
      setMissingJetons(updated.missingJetons ?? 0)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6 pt-4">
      <h1 className="font-primary text-2xl">Einstellungen</h1>

      {error && (
        <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2">
          {error}
        </div>
      )}

      <Card className="rounded-2xl">
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Food System</CardTitle>
              <p className="text-muted-foreground text-sm">
                Aktiviere oder deaktiviere das gesamte System für Kunden, POS und Stationen.
              </p>
            </div>
            <Switch
              checked={systemEnabled}
              onCheckedChange={(checked) => {
                setSystemEnabled(checked)
                updateSettings({ systemEnabled: checked })
              }}
              disabled={saving}
            />
          </div>
        </CardHeader>
      </Card>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>POS Modus</CardTitle>
          <p className="text-muted-foreground text-sm">
            Bestimme, ob das POS Belege mit QR-Code druckt, Jetons zum Ausgeben berechnet, oder beides kombiniert.
          </p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-3">
          <div className="grid gap-3 md:grid-cols-3">
            <Button
              variant={posMode === "QR_CODE" ? "default" : "outline"}
              className="h-12 justify-start rounded-xl"
              onClick={() => updateSettings({ posMode: "QR_CODE" })}
              disabled={saving}
            >
              QR-Code
            </Button>
            <Button
              variant={posMode === "JETON" ? "default" : "outline"}
              className="h-12 justify-start rounded-xl"
              onClick={() => updateSettings({ posMode: "JETON" })}
              disabled={saving}
            >
              Jetons
            </Button>
            <Button
              variant={posMode === "HYBRID" ? "default" : "outline"}
              className="h-12 justify-start rounded-xl"
              onClick={() => updateSettings({ posMode: "HYBRID" })}
              disabled={saving}
            >
              Hybrid
            </Button>
          </div>
          {missingJetons > 0 && (
            <div className="text-sm text-amber-700">
              {missingJetons} aktive Produkte haben keinen Jeton.{" "}
              <Link href="/admin/jetons" className="underline">
                Jetons verwalten
              </Link>
            </div>
          )}
        </CardContent>
      </Card>

      <PluginSlot name="admin.settings.section" ctx={{ token }} />
    </div>
  )
}
