"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"

type TenantInfo = { id: string; slug: string; name: string }

export default function AccountOverviewPage() {
  const fetchAuth = useAuthorizedFetch()
  const [tenant, setTenant] = useState<TenantInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    ;(async () => {
      try {
        const res = await fetchAuth("/api/v1/account/tenant")
        if (!res.ok) throw new Error(`Fehler ${res.status}`)
        setTenant((await res.json()) as TenantInfo)
      } catch (e) {
        setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
      } finally {
        setLoading(false)
      }
    })()
  }, [fetchAuth])

  // The day-to-day ops admin lives on the tenant's subdomain, derived from the
  // platform host we're on (env-agnostic: localhost / staging / prod).
  const opsBaseUrl =
    typeof window !== "undefined" && tenant ? `${window.location.protocol}//${tenant.slug}.${window.location.host}` : ""

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <h1 className="font-primary text-2xl">
          {loading ? "Laden…" : `Willkommen${tenant ? `, ${tenant.name}` : ""}`}
        </h1>
        <p className="text-muted-foreground text-sm">Verwalte hier deinen Auftritt, deine Domains und dein Konto.</p>
      </div>

      {error && (
        <div
          role="alert"
          className="text-destructive bg-destructive/10 rounded-[var(--radius-control)] px-3 py-2 text-sm"
        >
          {error}
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Tagesgeschäft</CardTitle>
          <p className="text-muted-foreground text-sm">
            Produkte, Bestellungen und Helfer verwaltest du im Betriebs-Admin auf deiner eigenen Subdomain.
          </p>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-3">
          {tenant ? (
            <>
              <Button asChild>
                <a href={`${opsBaseUrl}/admin`} target="_blank" rel="noopener noreferrer">
                  Betriebs-Admin öffnen
                </a>
              </Button>
              <Button asChild variant="outline">
                <a href={`${opsBaseUrl}/food`} target="_blank" rel="noopener noreferrer">
                  Storefront ansehen
                </a>
              </Button>
            </>
          ) : (
            <p className="text-muted-foreground text-sm">Laden…</p>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Marke &amp; Rechtliches</CardTitle>
            <p className="text-muted-foreground text-sm">Logo, Favicon, Farben, Kontakt und rechtliche Angaben.</p>
          </CardHeader>
          <CardContent>
            <Button asChild variant="outline">
              <Link href="/account/branding">Marke bearbeiten</Link>
            </Button>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Eigene Domains</CardTitle>
            <p className="text-muted-foreground text-sm">Verbinde eine eigene Domain mit deinem Auftritt.</p>
          </CardHeader>
          <CardContent>
            <Button asChild variant="outline">
              <Link href="/account/domains">Domains verwalten</Link>
            </Button>
          </CardContent>
        </Card>
      </div>

      {tenant && (
        <p className="text-muted-foreground text-xs">
          Subdomain:{" "}
          <span className="font-mono">
            {tenant.slug}.{typeof window !== "undefined" ? window.location.host : "comanda.leys.ch"}
          </span>
        </p>
      )}
    </div>
  )
}
