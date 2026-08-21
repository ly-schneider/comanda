"use client"

import { useCallback, useEffect, useState } from "react"
import QRCode from "@/components/qrcode"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { tenantSubdomainHost } from "@/lib/tenant"

type FetchAuth = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

type InstallInfo = {
  packageId: string
  channel: string
  installPath: string
  token?: string
  versionName?: string
}

type Channel = "production" | "staging"

const ERRORS: Record<string, string> = {
  no_android_package: "Diesem Tenant ist keine Android-Paket-ID zugewiesen.",
  invalid_channel: "Unbekannter Kanal.",
}

export function TenantAndroidInstallCard({
  tenantId,
  slug,
  fetchAuth,
}: {
  tenantId: string
  slug: string
  fetchAuth: FetchAuth
}) {
  const [channel, setChannel] = useState<Channel>("production")
  const [info, setInfo] = useState<InstallInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetchAuth(`/api/v1/platform/tenants/${tenantId}/android-install?channel=${channel}`)
      const body = (await res.json().catch(() => null)) as (InstallInfo & { code?: string }) | null
      if (!res.ok) {
        setInfo(null)
        setError((body?.code && ERRORS[body.code]) || "Laden fehlgeschlagen.")
        return
      }
      setInfo(body as InstallInfo)
    } catch {
      setInfo(null)
      setError("Laden fehlgeschlagen.")
    } finally {
      setLoading(false)
    }
  }, [channel, fetchAuth, tenantId])

  useEffect(() => {
    void load()
  }, [load])

  // The tenant's own host, not the platform host this page is served from.
  const installUrl =
    info && typeof window !== "undefined"
      ? `${window.location.protocol}//${tenantSubdomainHost(slug, window.location.host)}${info.installPath}`
      : null

  return (
    <Card>
      <CardHeader>
        <CardTitle>POS-App installieren</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex gap-2">
          {(["production", "staging"] as const).map((c) => (
            <Button
              key={c}
              type="button"
              size="sm"
              variant={channel === c ? "default" : "outline"}
              onClick={() => setChannel(c)}
            >
              {c === "production" ? "Produktion" : "Staging"}
            </Button>
          ))}
        </div>

        {loading && <p className="text-muted-foreground text-sm">Wird geladen…</p>}
        {error && <p className="text-destructive text-sm">{error}</p>}

        {installUrl && (
          <>
            <div className="flex justify-center">
              <QRCode value={installUrl} size={200} />
            </div>

            <div className="space-y-1">
              <Label className="text-xs">Installations-Link</Label>
              <div className="flex gap-2">
                <code className="bg-muted min-w-0 flex-1 truncate rounded-[var(--radius-control)] px-2 py-1.5 text-xs">
                  {installUrl}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void navigator.clipboard.writeText(installUrl).then(() => {
                      setCopied(true)
                      setTimeout(() => setCopied(false), 1500)
                    })
                  }}
                >
                  {copied ? "Kopiert" : "Kopieren"}
                </Button>
              </div>
            </div>

            <p className="text-muted-foreground text-xs">
              Mit der Tablet-Kamera scannen. Paket-ID: <code>{info?.packageId}</code>. Version:{" "}
              {info?.versionName ? (
                <code>{info.versionName}</code>
              ) : (
                <span className="text-destructive">noch kein Build — diesen Tenant beim nächsten Release mitgeben</span>
              )}
              .
              {info?.token
                ? " Der Link enthält das Download-Token — wie ein Passwort behandeln."
                : " Kein Download-Token konfiguriert; der Download ist offen erreichbar."}
            </p>
          </>
        )}
      </CardContent>
    </Card>
  )
}
