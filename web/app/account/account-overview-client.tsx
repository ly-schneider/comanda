"use client"

import { Check } from "lucide-react"
import Link from "next/link"
import { useCallback, useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import {
  completeOnboarding,
  getOnboardingStatus,
  getTenant,
  type OnboardingStatus,
  type TenantInfo,
} from "@/lib/api/account"

type Step = {
  key: string
  label: string
  description: string
  href: string
  done: boolean
  optional?: boolean
}

export default function AccountOverviewClient() {
  const fetchAuth = useAuthorizedFetch()
  const [tenant, setTenant] = useState<TenantInfo | null>(null)
  const [onboarding, setOnboarding] = useState<OnboardingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [finishing, setFinishing] = useState(false)

  const load = useCallback(async () => {
    try {
      const [tenantData, onbData] = await Promise.all([getTenant(fetchAuth), getOnboardingStatus(fetchAuth)])
      setTenant(tenantData)
      setOnboarding(onbData)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
    } finally {
      setLoading(false)
    }
  }, [fetchAuth])

  useEffect(() => {
    load()
  }, [load])

  async function finishOnboarding() {
    setFinishing(true)
    try {
      const ok = await completeOnboarding(fetchAuth)
      if (ok) {
        setOnboarding((prev) => (prev ? { ...prev, completedAt: new Date().toISOString() } : prev))
      }
    } finally {
      setFinishing(false)
    }
  }

  // The day-to-day ops admin lives on the tenant's subdomain, derived from the
  // platform host we're on (env-agnostic: localhost / staging / prod).
  const opsBaseUrl =
    typeof window !== "undefined" && tenant ? `${window.location.protocol}//${tenant.slug}.${window.location.host}` : ""

  const steps: Step[] = [
    {
      key: "tenant",
      label: "Konto erstellt",
      description: "Dein Auftritt ist live — Name und Subdomain sind eingerichtet.",
      href: "",
      done: true,
    },
    {
      key: "brand",
      label: "Marke & Rechtliches",
      description: "Lade dein Logo hoch und hinterlege Farben, Kontakt und rechtliche Angaben.",
      href: "/account/branding",
      done: onboarding?.steps.brand ?? false,
    },
    {
      key: "payments",
      label: "Zahlungen verbinden",
      description: "Verbinde dein Payrexx-Konto, damit Kunden online bezahlen können.",
      href: "/account/payrexx",
      done: onboarding?.steps.payments ?? false,
    },
    {
      key: "domain",
      label: "Eigene Domain",
      description: "Optional: Verknüpfe eine eigene Domain mit deinem Auftritt.",
      href: "/account/domains",
      done: onboarding?.steps.domain ?? false,
      optional: true,
    },
  ]

  const doneCount = steps.filter((s) => s.done).length
  const progressPct = Math.round((doneCount / steps.length) * 100)
  const allDone = doneCount === steps.length
  const dismissed = !!onboarding?.completedAt
  const showWizard = !!onboarding && !dismissed && !allDone

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

      {showWizard && (
        <Card>
          <CardHeader>
            <CardTitle>Einrichtung</CardTitle>
            <p className="text-muted-foreground text-sm">
              Schliesse die Schritte ab, um das Beste aus deinem Auftritt zu holen. Du kannst jeden Schritt überspringen
              und später fortsetzen.
            </p>
            <div className="space-y-1.5 pt-2">
              <Progress value={progressPct} />
              <p className="text-muted-foreground text-xs">
                {doneCount} von {steps.length} Schritten erledigt
              </p>
            </div>
          </CardHeader>
          <CardContent className="space-y-2">
            {steps.map((step) => (
              <div
                key={step.key}
                className="border-border flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-control)] border px-3 py-3"
              >
                <div className="flex items-start gap-3">
                  <span
                    className={`mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full text-xs ${
                      step.done ? "bg-primary text-primary-foreground" : "border-border border"
                    }`}
                    aria-hidden
                  >
                    {step.done && <Check className="size-3.5" />}
                  </span>
                  <div className="space-y-0.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{step.label}</span>
                      {step.optional && (
                        <Badge variant="outline" className="text-xs">
                          Optional
                        </Badge>
                      )}
                      {step.done && (
                        <Badge variant="secondary" className="text-xs">
                          Erledigt
                        </Badge>
                      )}
                    </div>
                    <p className="text-muted-foreground text-sm">{step.description}</p>
                  </div>
                </div>
                {step.href && (
                  <Button asChild variant={step.done ? "outline" : "primary"} size="sm">
                    <Link href={step.href}>{step.done ? "Bearbeiten" : "Einrichten"}</Link>
                  </Button>
                )}
              </div>
            ))}
            <div className="flex flex-wrap items-center justify-between gap-2 pt-2">
              <p className="text-muted-foreground text-xs">
                Pflicht für den Live-Betrieb sind nur Name und Subdomain — bereits erledigt.
              </p>
              <Button variant="ghost" size="sm" onClick={finishOnboarding} disabled={finishing}>
                {finishing ? "Speichern…" : allDone ? "Fertig" : "Onboarding abschliessen"}
              </Button>
            </div>
          </CardContent>
        </Card>
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

      <div className="grid gap-4 sm:grid-cols-3">
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
            <CardTitle>Zahlungen</CardTitle>
            <p className="text-muted-foreground text-sm">Verbinde dein Payrexx-Konto für Online-Zahlungen.</p>
          </CardHeader>
          <CardContent>
            <Button asChild variant="outline">
              <Link href="/account/payrexx">Zahlungen verwalten</Link>
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
