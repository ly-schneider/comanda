"use client"

import { useRouter } from "next/navigation"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

// Mirrors the backend slug rules (service/tenant.go) for fast client feedback;
// the backend remains authoritative (incl. the reserved-slug list).
const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

export default function NewTenantPage() {
  const router = useRouter()
  const fetchAuth = useAuthorizedFetch()
  const [slug, setSlug] = useState("")
  const [name, setName] = useState("")
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    setError(null)
    if (!name.trim()) {
      setError("Name erforderlich")
      return
    }
    if (!SLUG_RE.test(slug)) {
      setError("Ungültiger Slug (Kleinbuchstaben, Ziffern, Bindestriche)")
      return
    }
    setSaving(true)
    try {
      const res = await fetchAuth("/api/v1/platform/tenants", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
        body: JSON.stringify({ slug, name }),
      })
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { message?: string; detail?: string }
        throw new Error(j.message || j.detail || `Error ${res.status}`)
      }
      const t = (await res.json()) as { id: string }
      router.push(`/platform-admin/tenants/${t.id}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Erstellen fehlgeschlagen")
      setSaving(false)
    }
  }

  return (
    <div className="max-w-lg space-y-6">
      <h1 className="font-primary text-2xl">Tenant erstellen</h1>
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
          <CardTitle>Neuer Tenant</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Acme Bistro" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="slug">Slug</Label>
            <Input
              id="slug"
              value={slug}
              onChange={(e) => setSlug(e.target.value.toLowerCase().trim())}
              placeholder="acme"
              maxLength={63}
            />
            <p className="text-muted-foreground text-xs">Subdomain: {slug || "<slug>"}.comanda.leys.ch</p>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => router.back()} disabled={saving}>
              Abbrechen
            </Button>
            <Button onClick={submit} disabled={saving}>
              {saving ? "Erstellen…" : "Erstellen"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
