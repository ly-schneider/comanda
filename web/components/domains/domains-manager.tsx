"use client"

import { Check, ClipboardCopy, Info, RefreshCw, Trash2 } from "lucide-react"
import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type CertStatus = "none" | "pending" | "active" | "failed"

type Domain = {
  id: string
  hostname: string
  isPrimary: boolean
  verifiedAt: string | null
  certStatus: CertStatus
  createdAt: string
}

type DnsInstructions = {
  cnameTarget: string
  txtName: string
  txtValue: string
}

type AddDomainResult = {
  domain: Domain
} & DnsInstructions

function CertBadge({ status }: { status: CertStatus }) {
  const map: Record<CertStatus, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
    none: { label: "Kein Zertifikat", variant: "outline" },
    pending: { label: "Zertifikat ausstehend", variant: "secondary" },
    active: { label: "Zertifikat aktiv", variant: "default" },
    failed: { label: "Zertifikat fehlgeschlagen", variant: "destructive" },
  }
  const { label, variant } = map[status]
  return <Badge variant={variant}>{label}</Badge>
}

function VerifiedBadge({ verifiedAt }: { verifiedAt: string | null }) {
  if (verifiedAt) {
    return <Badge variant="default">Verifiziert</Badge>
  }
  return <Badge variant="outline">Nicht verifiziert</Badge>
}

function CopyField({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false)

  function copy() {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div className="space-y-1">
      <Label className="text-muted-foreground text-xs">{label}</Label>
      <div className="flex items-center gap-2">
        <code className="bg-muted flex-1 truncate rounded-lg px-3 py-1.5 font-mono text-sm">{value}</code>
        <Button variant="outline" size="sm" className="rounded-xl" onClick={copy} aria-label={`${label} kopieren`}>
          {copied ? <Check className="size-4" /> : <ClipboardCopy className="size-4" />}
        </Button>
      </div>
    </div>
  )
}

// DomainsManager is the shared custom-domain add/verify UI used by both the
// subdomain ops admin (/admin/domains → /api/v1/domains) and the platform
// account hub (/account/domains → /api/v1/account/domains). Only the API base
// path differs; the backend rebinds it to the right tenant.
export function DomainsManager({ basePath, heading = "Eigene Domains" }: { basePath: string; heading?: string }) {
  const fetchAuth = useAuthorizedFetch()

  const [domains, setDomains] = useState<Domain[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [hostname, setHostname] = useState("")
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState<string | null>(null)
  const [addResult, setAddResult] = useState<AddDomainResult | null>(null)

  const [verifying, setVerifying] = useState<Record<string, boolean>>({})
  const [verifyError, setVerifyError] = useState<Record<string, string>>({})
  const [removing, setRemoving] = useState<Record<string, boolean>>({})
  const [removeError, setRemoveError] = useState<Record<string, string>>({})
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null)

  const [dnsLoading, setDnsLoading] = useState<Record<string, boolean>>({})
  const [dnsInstructions, setDnsInstructions] = useState<Record<string, DnsInstructions>>({})

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const res = await fetchAuth(basePath)
      if (!res.ok) throw new Error(`Fehler ${res.status}`)
      const data = (await res.json()) as { items: Domain[] }
      setDomains(data.items ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    ;(async () => {
      await load()
    })()
  }, [fetchAuth, basePath])

  async function addDomain(e: React.FormEvent) {
    e.preventDefault()
    setAdding(true)
    setAddError(null)
    setAddResult(null)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(basePath, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({ hostname: hostname.trim().toLowerCase() }),
      })
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { code?: string; message?: string; detail?: string }
        if (j.code === "invalid_hostname") throw new Error("Ungültiger Hostname. Bitte gib eine gültige Domain ein.")
        if (j.code === "platform_subdomain") throw new Error("Plattform-Subdomains können nicht registriert werden.")
        if (res.status === 409) throw new Error("Diese Domain ist bereits registriert.")
        throw new Error(j.message || j.detail || `Fehler ${res.status}`)
      }
      const result = (await res.json()) as AddDomainResult
      setAddResult(result)
      setDomains((prev) => [result.domain, ...prev])
      setHostname("")
    } catch (e) {
      setAddError(e instanceof Error ? e.message : "Hinzufügen fehlgeschlagen")
    } finally {
      setAdding(false)
    }
  }

  async function verify(id: string) {
    setVerifying((prev) => ({ ...prev, [id]: true }))
    setVerifyError((prev) => {
      const n = { ...prev }
      delete n[id]
      return n
    })
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`${basePath}/${id}/verify`, {
        method: "POST",
        headers: { "X-CSRF": csrf || "" },
      })
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { message?: string; detail?: string }
        throw new Error(j.message || j.detail || `Prüfung fehlgeschlagen (${res.status})`)
      }
      const updated = (await res.json()) as Domain
      setDomains((prev) => prev.map((d) => (d.id === id ? updated : d)))
    } catch (e) {
      setVerifyError((prev) => ({ ...prev, [id]: e instanceof Error ? e.message : "Prüfung fehlgeschlagen" }))
    } finally {
      setVerifying((prev) => ({ ...prev, [id]: false }))
    }
  }

  async function remove(id: string) {
    setRemoving((prev) => ({ ...prev, [id]: true }))
    setRemoveError((prev) => {
      const n = { ...prev }
      delete n[id]
      return n
    })
    setConfirmRemove(null)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`${basePath}/${id}`, {
        method: "DELETE",
        headers: { "X-CSRF": csrf || "" },
      })
      if (res.ok || res.status === 204) {
        setDomains((prev) => prev.filter((d) => d.id !== id))
        if (addResult?.domain.id === id) setAddResult(null)
        setDnsInstructions((prev) => {
          const n = { ...prev }
          delete n[id]
          return n
        })
      } else {
        const j = (await res.json().catch(() => ({}))) as { message?: string; detail?: string }
        throw new Error(j.message || j.detail || `Entfernen fehlgeschlagen (${res.status})`)
      }
    } catch (e) {
      setRemoveError((prev) => ({ ...prev, [id]: e instanceof Error ? e.message : "Entfernen fehlgeschlagen" }))
    } finally {
      setRemoving((prev) => ({ ...prev, [id]: false }))
    }
  }

  async function loadDnsInstructions(id: string) {
    if (dnsInstructions[id]) {
      setDnsInstructions((prev) => {
        const n = { ...prev }
        delete n[id]
        return n
      })
      return
    }
    setDnsLoading((prev) => ({ ...prev, [id]: true }))
    try {
      const res = await fetchAuth(`${basePath}/${id}`)
      if (!res.ok) throw new Error(`Fehler ${res.status}`)
      const data = (await res.json()) as { domain: Domain; cnameTarget: string; txtName: string; txtValue: string }
      setDnsInstructions((prev) => ({
        ...prev,
        [id]: { cnameTarget: data.cnameTarget, txtName: data.txtName, txtValue: data.txtValue },
      }))
    } catch {
      // silently ignore — button stays available to retry
    } finally {
      setDnsLoading((prev) => ({ ...prev, [id]: false }))
    }
  }

  return (
    <div className="space-y-6 pt-4">
      <h1 className="font-primary text-2xl">{heading}</h1>

      {error && (
        <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2">
          {error}
        </div>
      )}

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Domain hinzufügen</CardTitle>
          <p className="text-muted-foreground text-sm">
            Verknüpfe eine eigene Domain mit deinem Tenant. Du wirst danach die nötigen DNS-Einträge einrichten müssen.
          </p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          {addError && (
            <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
              {addError}
            </div>
          )}
          <form onSubmit={addDomain} className="flex gap-2">
            <div className="flex-1 space-y-1">
              <Label htmlFor="newHostname" className="sr-only">
                Hostname
              </Label>
              <Input
                id="newHostname"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
                placeholder="shop.example.com"
                className="rounded-xl"
                disabled={adding}
                autoComplete="off"
                spellCheck={false}
              />
            </div>
            <Button type="submit" disabled={adding || !hostname.trim()} className="h-9 rounded-xl">
              {adding ? "Hinzufügen…" : "Hinzufügen"}
            </Button>
          </form>

          {addResult && (
            <div className="bg-muted space-y-4 rounded-xl p-4">
              <p className="text-sm font-medium">
                Domain registriert. Erstelle die folgenden DNS-Einträge und klicke dann auf{" "}
                <strong>Jetzt prüfen</strong>.
              </p>
              <CopyField label="CNAME-Ziel" value={addResult.cnameTarget} />
              <CopyField label="TXT-Eintragsname" value={addResult.txtName} />
              <CopyField label="TXT-Wert" value={addResult.txtValue} />
              <p className="text-muted-foreground text-xs">
                DNS-Änderungen können bis zu 48 Stunden dauern. Die automatische Prüfung erfolgt regelmässig im
                Hintergrund.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Registrierte Domains</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-muted-foreground text-sm">Laden…</p>
          ) : domains.length === 0 ? (
            <p className="text-muted-foreground text-sm">Noch keine Domains registriert.</p>
          ) : (
            <div className="divide-y">
              {domains.map((domain) => {
                const dns = dnsInstructions[domain.id]
                return (
                  <div key={domain.id} className="space-y-2 py-4 first:pt-0">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="space-y-1.5">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm font-medium">{domain.hostname}</span>
                          {domain.isPrimary && (
                            <Badge variant="secondary" className="text-xs">
                              Primär
                            </Badge>
                          )}
                        </div>
                        <div className="flex flex-wrap gap-1.5">
                          <VerifiedBadge verifiedAt={domain.verifiedAt} />
                          <CertBadge status={domain.certStatus} />
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        {!domain.verifiedAt && (
                          <>
                            <Button
                              variant="outline"
                              size="sm"
                              className="rounded-xl"
                              onClick={() => loadDnsInstructions(domain.id)}
                              disabled={dnsLoading[domain.id]}
                              aria-label="DNS-Einträge anzeigen"
                            >
                              <Info className="size-4" />
                              DNS-Einträge
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              className="rounded-xl"
                              onClick={() => verify(domain.id)}
                              disabled={verifying[domain.id]}
                            >
                              <RefreshCw className={`size-4 ${verifying[domain.id] ? "animate-spin" : ""}`} />
                              Jetzt prüfen
                            </Button>
                          </>
                        )}
                        {confirmRemove === domain.id ? (
                          <div className="flex items-center gap-1.5">
                            <span className="text-muted-foreground text-xs">Wirklich?</span>
                            <Button
                              variant="destructive"
                              size="sm"
                              className="rounded-xl"
                              onClick={() => remove(domain.id)}
                              disabled={removing[domain.id]}
                            >
                              {removing[domain.id] ? "Entfernen…" : "Ja, entfernen"}
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              className="rounded-xl"
                              onClick={() => setConfirmRemove(null)}
                            >
                              Abbrechen
                            </Button>
                          </div>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            className="rounded-xl"
                            onClick={() => setConfirmRemove(domain.id)}
                            disabled={!!removing[domain.id]}
                            aria-label={`${domain.hostname} entfernen`}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        )}
                      </div>
                    </div>
                    {verifyError[domain.id] && (
                      <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
                        {verifyError[domain.id]}
                      </div>
                    )}
                    {removeError[domain.id] && (
                      <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
                        {removeError[domain.id]}
                      </div>
                    )}
                    {dns && (
                      <div className="bg-muted space-y-4 rounded-xl p-4">
                        <p className="text-sm font-medium">
                          Erstelle die folgenden DNS-Einträge und klicke dann auf <strong>Jetzt prüfen</strong>.
                        </p>
                        <CopyField label="CNAME-Ziel" value={dns.cnameTarget} />
                        <CopyField label="TXT-Eintragsname" value={dns.txtName} />
                        <CopyField label="TXT-Wert" value={dns.txtValue} />
                        <p className="text-muted-foreground text-xs">
                          DNS-Änderungen können bis zu 48 Stunden dauern. Die automatische Prüfung erfolgt regelmässig
                          im Hintergrund.
                        </p>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
