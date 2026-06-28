"use client"

import { Check, ClipboardCopy } from "lucide-react"
import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { PasswordInput } from "@/components/ui/password-input"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type PayrexxState = "not_started" | "api_connected" | "configured"

type PayrexxStatus = {
  state: PayrexxState
  instance?: string
  webhookUrl?: string
  lastWebhookAt?: string
}

function CopyableCode({ value, ariaLabel }: { value: string; ariaLabel: string }) {
  const [copied, setCopied] = useState(false)
  function copy() {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }
  return (
    <div className="flex items-center gap-2">
      <code className="bg-background flex-1 truncate rounded-lg px-3 py-1.5 font-mono text-sm">{value}</code>
      <Button variant="outline" size="sm" className="rounded-xl" onClick={copy} aria-label={ariaLabel}>
        {copied ? <Check className="size-4" /> : <ClipboardCopy className="size-4" />}
      </Button>
    </div>
  )
}

function ValueRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[8rem_1fr] items-center gap-2">
      <span className="text-muted-foreground text-sm">{label}</span>
      <span className="text-sm font-medium">{children}</span>
    </div>
  )
}

async function errorMessage(res: Response, fallback: string) {
  const j = (await res.json().catch(() => ({}))) as { message?: string; detail?: string }
  return j.message || j.detail || fallback
}

// PayrexxBindingForm is the shared per-tenant Payrexx connection editor, split
// into two stages that mirror how Payrexx works: first connect the API (instance
// + API key, validated live), then register the webhook — Payrexx only generates
// the webhook signing secret after the merchant creates the webhook from the URL
// shown here. Secrets are write-only; the GET status never returns them.
export function PayrexxBindingForm({ basePath, heading = "Zahlungen" }: { basePath: string; heading?: string }) {
  const fetchAuth = useAuthorizedFetch()

  const [status, setStatus] = useState<PayrexxStatus | null>(null)
  const [instance, setInstance] = useState("")
  const [apiSecret, setApiSecret] = useState("")
  const [webhookSecret, setWebhookSecret] = useState("")

  const [connecting, setConnecting] = useState(false)
  const [apiError, setApiError] = useState<string | null>(null)
  const [apiSuccess, setApiSuccess] = useState(false)

  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<"ok" | "fail" | null>(null)

  const [savingWebhook, setSavingWebhook] = useState(false)
  const [webhookError, setWebhookError] = useState<string | null>(null)
  const [webhookSuccess, setWebhookSuccess] = useState(false)

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

  const state = status?.state ?? "not_started"
  const apiConnected = state === "api_connected" || state === "configured"
  const configured = state === "configured"

  async function connectApi(e: React.FormEvent) {
    e.preventDefault()
    setConnecting(true)
    setApiError(null)
    setApiSuccess(false)
    setTestResult(null)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`${basePath}/api`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({ instance: instance.trim(), apiSecret: apiSecret.trim() }),
      })
      if (!res.ok) {
        throw new Error(
          await errorMessage(res, "Payrexx-Zugangsdaten konnten nicht überprüft werden. Prüfe Instanz und API-Key.")
        )
      }
      setStatus((await res.json()) as PayrexxStatus)
      setApiSecret("")
      setApiSuccess(true)
      setTimeout(() => setApiSuccess(false), 4000)
    } catch (err) {
      setApiError(err instanceof Error ? err.message : "Speichern fehlgeschlagen")
    } finally {
      setConnecting(false)
    }
  }

  async function testConnection() {
    setTesting(true)
    setTestResult(null)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`${basePath}/test`, {
        method: "POST",
        headers: { "X-CSRF": csrf || "" },
      })
      setTestResult(res.ok ? "ok" : "fail")
    } catch {
      setTestResult("fail")
    } finally {
      setTesting(false)
    }
  }

  async function saveWebhook(e: React.FormEvent) {
    e.preventDefault()
    setSavingWebhook(true)
    setWebhookError(null)
    setWebhookSuccess(false)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`${basePath}/webhook`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({ webhookSecret: webhookSecret.trim() }),
      })
      if (!res.ok) {
        throw new Error(await errorMessage(res, "Webhook-Secret konnte nicht gespeichert werden."))
      }
      setStatus((await res.json()) as PayrexxStatus)
      setWebhookSecret("")
      setWebhookSuccess(true)
      setTimeout(() => setWebhookSuccess(false), 4000)
    } catch (err) {
      setWebhookError(err instanceof Error ? err.message : "Speichern fehlgeschlagen")
    } finally {
      setSavingWebhook(false)
    }
  }

  return (
    <div className="space-y-6 pt-4">
      <div className="flex items-center gap-3">
        <h1 className="font-primary text-2xl">{heading}</h1>
        {configured && <Badge variant="default">Verbunden</Badge>}
      </div>

      {/* Step 1 — API access */}
      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Schritt 1: API-Zugang</CardTitle>
          <p className="text-muted-foreground text-sm">
            Verbinde dein Payrexx-Konto, damit Kunden online bezahlen können. Instanzname und API-Key findest du in
            deinem Payrexx-Dashboard unter <strong>API &amp; Plugins</strong>.
          </p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          {apiError && (
            <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
              {apiError}
            </div>
          )}
          {apiSuccess && (
            <div role="status" className="rounded bg-green-100 px-3 py-2 text-sm text-green-800">
              API-Zugang verbunden
            </div>
          )}

          <form onSubmit={connectApi} className="space-y-4">
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
                disabled={connecting}
                required
              />
              <p className="text-muted-foreground text-xs">
                Der Teil vor <span className="font-mono">.payrexx.com</span>.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="payrexx-api-secret">API-Key</Label>
              <PasswordInput
                id="payrexx-api-secret"
                value={apiSecret}
                onChange={(e) => setApiSecret(e.target.value)}
                placeholder={apiConnected ? "••••••••  (zum Ändern neu eingeben)" : ""}
                className="rounded-xl font-mono"
                autoComplete="off"
                disabled={connecting}
                required
              />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex items-center gap-3 text-sm">
                {apiConnected && (
                  <span className="inline-flex items-center gap-1 text-green-700">
                    <Check className="size-4" /> Verbunden
                  </span>
                )}
                {apiConnected && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="rounded-xl"
                    onClick={testConnection}
                    disabled={testing}
                  >
                    {testing ? "Teste…" : "Erneut testen"}
                  </Button>
                )}
                {testResult === "ok" && <span className="text-green-700">Verbindung erfolgreich</span>}
                {testResult === "fail" && <span className="text-destructive">Verbindung fehlgeschlagen</span>}
              </div>
              <Button type="submit" disabled={connecting} className="h-11 min-w-32 rounded-xl">
                {connecting ? "Prüfe…" : apiConnected ? "Aktualisieren" : "Verbinden"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Step 2 — Webhook */}
      <Card className={`rounded-2xl ${apiConnected ? "" : "pointer-events-none opacity-50"}`}>
        <CardHeader>
          <CardTitle>Schritt 2: Webhook einrichten</CardTitle>
          <p className="text-muted-foreground text-sm">
            {apiConnected
              ? "Erstelle in Payrexx einen Webhook mit den folgenden Werten. Payrexx zeigt dir danach ein Signatur-Secret, das du hier einträgst."
              : "Schliesse zuerst Schritt 1 ab."}
          </p>
        </CardHeader>
        {apiConnected && (
          <CardContent className="space-y-5">
            <div className="bg-muted space-y-3 rounded-xl p-4">
              <p className="text-sm">
                In Payrexx unter <strong>Webhooks → Webhook hinzufügen</strong>:
              </p>
              {status?.webhookUrl && (
                <div className="space-y-1.5">
                  <span className="text-muted-foreground text-sm">URL</span>
                  <CopyableCode value={status.webhookUrl} ariaLabel="Webhook-URL kopieren" />
                </div>
              )}
              <div className="space-y-2 border-t pt-3">
                <ValueRow label="Typ">JSON</ValueRow>
                <ValueRow label="Events">Transaction</ValueRow>
                <ValueRow label="Bei Fehler erneut versuchen">aktivieren</ValueRow>
                <ValueRow label="Version">neueste</ValueRow>
                <ValueRow label="Name">
                  Comanda <span className="text-muted-foreground font-normal">(frei wählbar)</span>
                </ValueRow>
              </div>
            </div>

            {webhookError && (
              <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
                {webhookError}
              </div>
            )}
            {webhookSuccess && (
              <div role="status" className="rounded bg-green-100 px-3 py-2 text-sm text-green-800">
                Webhook-Secret gespeichert
              </div>
            )}

            <form onSubmit={saveWebhook} className="max-w-lg space-y-4">
              <div className="space-y-2">
                <Label htmlFor="payrexx-webhook-secret">Webhook-Secret (von Payrexx)</Label>
                <PasswordInput
                  id="payrexx-webhook-secret"
                  value={webhookSecret}
                  onChange={(e) => setWebhookSecret(e.target.value)}
                  placeholder={configured ? "••••••••  (zum Ändern neu eingeben)" : ""}
                  className="rounded-xl font-mono"
                  autoComplete="off"
                  disabled={savingWebhook}
                  required
                />
              </div>
              <div className="flex justify-end">
                <Button type="submit" disabled={savingWebhook} className="h-11 min-w-32 rounded-xl">
                  {savingWebhook ? "Speichere…" : "Speichern"}
                </Button>
              </div>
            </form>

            {configured && (
              <div className="rounded-xl border p-4 text-sm">
                {status?.lastWebhookAt ? (
                  <span className="inline-flex items-center gap-2 text-green-700">
                    <Check className="size-4" />
                    Letzter Webhook empfangen:{" "}
                    {new Date(status.lastWebhookAt).toLocaleString("de-CH", {
                      dateStyle: "medium",
                      timeStyle: "short",
                    })}
                  </span>
                ) : (
                  <span className="text-muted-foreground">
                    Noch kein Webhook empfangen. Löse in Payrexx eine Test-Zahlung aus, um die Zustellung zu bestätigen.
                  </span>
                )}
              </div>
            )}
          </CardContent>
        )}
      </Card>
    </div>
  )
}
