"use client"

import { Check } from "lucide-react"
import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type SettingsResponse = { sumupMerchantCode?: string | null }

async function errorMessage(res: Response, fallback: string) {
  const j = (await res.json().catch(() => ({}))) as { message?: string; detail?: string }
  return j.message || j.detail || fallback
}

// SumUpSettingsForm edits the tenant's expected SumUp merchant code — the account
// its POS tablets must be logged into. It is a non-secret identifier, so it rides
// the shared account-hub settings endpoint (GET/PATCH .../settings); sending an
// empty string clears it and turns the on-device guardrail back to warn-only.
export function SumUpSettingsForm({ basePath }: { basePath: string }) {
  const fetchAuth = useAuthorizedFetch()

  const [code, setCode] = useState("")
  const [saved, setSaved] = useState("")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  useEffect(() => {
    let active = true
    ;(async () => {
      try {
        const res = await fetchAuth(basePath)
        if (res.ok && active) {
          const data = (await res.json()) as SettingsResponse
          const value = data.sumupMerchantCode ?? ""
          setCode(value)
          setSaved(value)
        }
      } finally {
        if (active) setLoading(false)
      }
    })()
    return () => {
      active = false
    }
  }, [fetchAuth, basePath])

  const dirty = code.trim() !== saved

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError(null)
    setSuccess(false)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(basePath, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({ sumupMerchantCode: code.trim() }),
      })
      if (!res.ok) {
        throw new Error(await errorMessage(res, "SumUp-Händler-Code konnte nicht gespeichert werden."))
      }
      const data = (await res.json()) as SettingsResponse
      const value = data.sumupMerchantCode ?? ""
      setCode(value)
      setSaved(value)
      setSuccess(true)
      setTimeout(() => setSuccess(false), 4000)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6 pt-4">
      <div className="flex items-center gap-3">
        <h1 className="font-primary text-2xl">SumUp</h1>
        {saved && <Badge variant="default">Aktiv</Badge>}
      </div>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Kartenzahlung (SumUp)</CardTitle>
          <p className="text-muted-foreground text-sm">
            Hinterlege den SumUp-Händler-Code, auf den deine Kassen-Tablets angemeldet sein müssen. Ist ein Tablet bei
            einem anderen SumUp-Konto angemeldet, werden Kartenzahlungen auf der Kasse blockiert.
          </p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          {error && (
            <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
              {error}
            </div>
          )}
          {success && (
            <div role="status" className="rounded bg-green-100 px-3 py-2 text-sm text-green-800">
              Gespeichert
            </div>
          )}

          <form onSubmit={save} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="sumup-merchant-code">SumUp-Händler-Code</Label>
              <Input
                id="sumup-merchant-code"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder="z. B. MC7A2K9P"
                className="rounded-xl font-mono"
                autoComplete="off"
                spellCheck={false}
                disabled={loading || saving}
              />
              <p className="text-muted-foreground text-xs">
                Findest du unter <span className="font-mono">me.sumup.com</span> oben links unter deinem Geschäftsnamen
                (in der SumUp-API «merchant_code»).
              </p>
              <p className="text-muted-foreground text-xs">
                Leer lassen und speichern, um die Kontoprüfung zu deaktivieren — die Kassen warnen dann nur, statt
                Kartenzahlungen zu blockieren.
              </p>
            </div>
            <div className="flex items-center justify-end gap-3">
              {saved && !dirty && (
                <span className="inline-flex items-center gap-1 text-sm text-green-700">
                  <Check className="size-4" /> Hinterlegt
                </span>
              )}
              <Button type="submit" disabled={loading || saving || !dirty} className="h-11 min-w-32 rounded-xl">
                {saving ? "Speichere…" : "Speichern"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
