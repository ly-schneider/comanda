"use client"

import Link from "next/link"
import { useParams } from "next/navigation"
import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type Tenant = { id: string; slug: string; name: string; status: string; createdAt: string }
type OwnerInviteResult = { email: string; status: string; expiresAt: string } | null

const OWNER_INVITE_ERRORS: Record<string, string> = {
  owner_exists: "Für diesen Tenant existiert bereits ein Inhaber.",
  tenant_not_linked: "Tenant ist noch nicht mit einer Organisation verknüpft.",
  tenant_not_found: "Tenant nicht gefunden.",
  invalid_email: "Ungültige E-Mail-Adresse.",
}

export default function ManageTenantPage() {
  const params = useParams<{ id: string }>()
  const id = params?.id
  const fetchAuth = useAuthorizedFetch()
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [inviteEmail, setInviteEmail] = useState("")
  const [inviting, setInviting] = useState(false)
  const [inviteError, setInviteError] = useState<string | null>(null)
  const [inviteResult, setInviteResult] = useState<OwnerInviteResult>(null)

  const load = useCallback(async () => {
    if (!id) return
    try {
      const res = await fetchAuth(`/api/v1/platform/tenants/${id}`)
      if (!res.ok) throw new Error(`Error ${res.status}`)
      setTenant((await res.json()) as Tenant)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
    } finally {
      setLoading(false)
    }
  }, [fetchAuth, id])

  useEffect(() => {
    load()
  }, [load])

  async function setStatus(status: "active" | "suspended") {
    if (!id) return
    setBusy(true)
    setError(null)
    try {
      const res = await fetchAuth(`/api/v1/platform/tenants/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
        body: JSON.stringify({ status }),
      })
      if (!res.ok) throw new Error(`Error ${res.status}`)
      setTenant((await res.json()) as Tenant)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Aktualisieren fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }

  async function inviteOwner(e: React.FormEvent) {
    e.preventDefault()
    if (!id || !inviteEmail.trim()) return
    setInviting(true)
    setInviteError(null)
    setInviteResult(null)
    try {
      const res = await fetchAuth(`/api/v1/platform/tenants/${id}/owner-invite`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
        body: JSON.stringify({ email: inviteEmail.trim() }),
      })
      const body = (await res.json().catch(() => ({}))) as {
        code?: string
        email?: string
        status?: string
        expiresAt?: string
      }
      if (!res.ok) {
        const code = body.code || ""
        setInviteError(OWNER_INVITE_ERRORS[code] ?? "Einladung fehlgeschlagen.")
        return
      }
      setInviteResult({ email: body.email!, status: body.status!, expiresAt: body.expiresAt! })
      setInviteEmail("")
    } catch {
      setInviteError("Einladung fehlgeschlagen. Bitte erneut versuchen.")
    } finally {
      setInviting(false)
    }
  }

  if (loading) return <p className="text-muted-foreground text-sm">Laden…</p>
  if (!tenant) {
    return (
      <div className="space-y-4">
        <div
          role="alert"
          className="text-destructive bg-destructive/10 rounded-[var(--radius-control)] px-3 py-2 text-sm"
        >
          {error ?? "Tenant nicht gefunden"}
        </div>
        <Button asChild variant="outline">
          <Link href="/platform-admin">Zurück</Link>
        </Button>
      </div>
    )
  }

  // The tenant's storefront lives on its subdomain, derived from the platform
  // host the operator is on (env-agnostic: localhost / staging / prod).
  const tenantBaseUrl =
    typeof window !== "undefined" ? `${window.location.protocol}//${tenant.slug}.${window.location.host}` : ""
  const onboardingSteps = [
    { label: "Branding einrichten", href: `/t/${tenant.slug}/branding` },
    { label: "Payrexx verbinden", href: `/t/${tenant.slug}/payrexx` },
    { label: "Eigene Domain", href: `/t/${tenant.slug}/domains` },
  ]

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="font-primary text-2xl">{tenant.name}</h1>
        <Button asChild variant="outline" size="sm">
          <Link href="/platform-admin">Zurück</Link>
        </Button>
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
          <CardTitle>Übersicht</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <div className="flex justify-between">
            <span className="text-muted-foreground">Slug</span>
            <span className="font-mono">{tenant.slug}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Status</span>
            <span>{tenant.status}</span>
          </div>
          <div className="flex gap-2 pt-2">
            {tenant.status === "active" ? (
              <Button variant="destructive" size="sm" disabled={busy} onClick={() => setStatus("suspended")}>
                Sperren
              </Button>
            ) : (
              <Button variant="primary" size="sm" disabled={busy} onClick={() => setStatus("active")}>
                Aktivieren
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Inhaber einladen</CardTitle>
          <p className="text-muted-foreground text-sm">Sendet einen Registrierungslink an den Tenant-Inhaber.</p>
        </CardHeader>
        <CardContent>
          <form onSubmit={inviteOwner} className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="invite-email">E-Mail-Adresse</Label>
              <div className="flex gap-2">
                <Input
                  id="invite-email"
                  type="email"
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                  placeholder="inhaber@beispiel.ch"
                  disabled={inviting}
                  required
                />
                <Button type="submit" disabled={inviting || !inviteEmail.trim()}>
                  {inviting ? "Sende…" : "Einladen"}
                </Button>
              </div>
            </div>
            {inviteError && (
              <div
                role="alert"
                className="text-destructive bg-destructive/10 rounded-[var(--radius-control)] px-3 py-2 text-sm"
              >
                {inviteError}
              </div>
            )}
            {inviteResult && (
              <div className="rounded-[var(--radius-control)] bg-green-50 px-3 py-2 text-sm text-green-800 dark:bg-green-950 dark:text-green-200">
                Einladung gesendet an <span className="font-medium">{inviteResult.email}</span>
                {" · "}gültig bis{" "}
                {new Date(inviteResult.expiresAt).toLocaleString("de-CH", { dateStyle: "short", timeStyle: "short" })}
              </div>
            )}
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Onboarding</CardTitle>
          <p className="text-muted-foreground text-sm">Im Konto-Hub für diesen Tenant verwalten.</p>
        </CardHeader>
        <CardContent className="space-y-2">
          {onboardingSteps.map((step) => (
            <a
              key={step.href}
              href={step.href}
              target="_blank"
              rel="noreferrer"
              className="border-border hover:bg-accent flex items-center justify-between rounded-[var(--radius-control)] border px-3 py-2 text-sm transition-colors"
            >
              <span>{step.label}</span>
              <span className="text-muted-foreground">↗</span>
            </a>
          ))}
          <a
            href={`${tenantBaseUrl}/food`}
            target="_blank"
            rel="noreferrer"
            className="text-muted-foreground inline-block pt-1 text-sm underline-offset-4 hover:underline"
          >
            Storefront öffnen ↗
          </a>
        </CardContent>
      </Card>
    </div>
  )
}
