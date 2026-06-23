"use client"

import { Check, ClipboardCopy } from "lucide-react"
import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type PayrexxStatus = {
  configured: boolean
  instance?: string
  webhookUrl?: string
}

function WebhookUrl({ url }: { url: string }) {
  const [copied, setCopied] = useState(false)
  function copy() {
    navigator.clipboard.writeText(url).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }
  return (
    <div className="bg-muted space-y-2 rounded-xl p-4">
      <p className="text-sm font-medium">Webhook-URL</p>
      <p className="text-muted-foreground text-sm">
        Trage diese URL in deinem Payrexx-Dashboard unter <strong>Integrationen → Webhook</strong> ein, damit Zahlungen
        deinem Konto zugeordnet werden.
      </p>
      <div className="flex items-center gap-2">
        <code className="bg-background flex-1 truncate rounded-lg px-3 py-1.5 font-mono text-sm">{url}</code>
        <Button variant="outline" size="sm" className="rounded-xl" onClick={copy} aria-label="Webhook-URL kopieren">
          {copied ? <Check className="size-4" /> : <ClipboardCopy className="size-4" />}
        </Button>
      </div>
    </div>
  )
}

// PayrexxBindingForm is the shared per-tenant Payrexx connection editor. It binds
// the instance name + API secret + webhook secret against PUT {basePath}; the
// backend validates the credentials against Payrexx, encrypts the secrets
// (AES-256-GCM), and returns the tenant-scoped webhook URL to paste into Payrexx.
// Secrets are write-only — the GET status never returns them, so reconfiguring
// always requires re-entering all three.
export function PayrexxBindingForm({ basePath, heading = "Zahlungen" }: { basePath: string; heading?: string }) {
  const fetchAuth = useAuthorizedFetch()

  const [status, setStatus] = useState<PayrexxStatus | null>(null)
  const [instance, setInstance] = useState("")
  const [apiSecret, setApiSecret] = useState("")
  const [webhookSecret, setWebhookSecret] = useState("")

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  useEffect(() => {
    ;(async () => {
      const res = await fetchAuth(basePath)
      if (res.ok) {
        const data = (await res.json()) as PayrexxStatus
        setStatus(data)
        if (data.instance) setInstance(data.instance)
      }
    })()
  }, [fetchAuth, basePath])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError(null)
    setSuccess(false)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(basePath, {
        method: "PUT",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({
          instance: instance.trim(),
          apiSecret: apiSecret.trim(),
          webhookSecret: webhookSecret.trim(),
        }),
      })
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { message?: string; detail?: string }
        throw new Error(
          j.message || j.detail || "Payrexx-Zugangsdaten konnten nicht überprüft werden. Prüfe Instanz und API-Secret."
        )
      }
      const data = (await res.json()) as { configured: boolean; webhookUrl: string }
      setStatus({ configured: true, instance: instance.trim(), webhookUrl: data.webhookUrl })
      setApiSecret("")
      setWebhookSecret("")
      setSuccess(true)
      setTimeout(() => setSuccess(false), 4000)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6 pt-4">
      <div className="flex items-center gap-3">
        <h1 className="font-primary text-2xl">{heading}</h1>
        {status?.configured && <Badge variant="default">Verbunden</Badge>}
      </div>

      {error && (
        <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
          {error}
        </div>
      )}
      {success && (
        <div role="status" className="rounded bg-green-100 px-3 py-2 text-sm text-green-800">
          Payrexx verbunden
        </div>
      )}

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Payrexx verbinden</CardTitle>
          <p className="text-muted-foreground text-sm">
            Verbinde dein Payrexx-Konto, damit Kunden online bezahlen können. Instanzname und API-Secret findest du in
            deinem Payrexx-Dashboard unter <strong>Integrationen → API &amp; Plugins</strong>.
          </p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          <form onSubmit={save} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="payrexx-instance">Instanzname</Label>
              <Input
                id="payrexx-instance"
                value={instance}
                onChange={(e) => setInstance(e.target.value)}
                placeholder="meine-instanz"
                className="rounded-xl"
                autoComplete="off"
                spellCheck={false}
                disabled={saving}
                required
              />
              <p className="text-muted-foreground text-xs">
                Der Teil vor <span className="font-mono">.payrexx.com</span>.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="payrexx-api-secret">API-Secret</Label>
              <Input
                id="payrexx-api-secret"
                type="password"
                value={apiSecret}
                onChange={(e) => setApiSecret(e.target.value)}
                placeholder={status?.configured ? "••••••••  (zum Ändern neu eingeben)" : ""}
                className="rounded-xl font-mono"
                autoComplete="off"
                disabled={saving}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="payrexx-webhook-secret">Webhook-Secret</Label>
              <Input
                id="payrexx-webhook-secret"
                type="password"
                value={webhookSecret}
                onChange={(e) => setWebhookSecret(e.target.value)}
                placeholder={status?.configured ? "••••••••  (zum Ändern neu eingeben)" : ""}
                className="rounded-xl font-mono"
                autoComplete="off"
                disabled={saving}
                required
              />
            </div>
            <div className="flex justify-end">
              <Button type="submit" disabled={saving} className="h-11 min-w-32 rounded-xl">
                {saving ? "Prüfe…" : status?.configured ? "Aktualisieren" : "Verbinden"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {status?.configured && status.webhookUrl && (
        <Card className="rounded-2xl">
          <CardHeader>
            <CardTitle>Webhook einrichten</CardTitle>
          </CardHeader>
          <CardContent>
            <WebhookUrl url={status.webhookUrl} />
          </CardContent>
        </Card>
      )}
    </div>
  )
}
