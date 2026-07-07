"use client"

import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"
import { readApiError } from "@/lib/http"

const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

type FieldKey = "name" | "slug"

export default function NewTenantPage() {
  const router = useRouter()
  const fetchAuth = useAuthorizedFetch()
  const [slug, setSlug] = useState("")
  const [name, setName] = useState("")
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<FieldKey, string>>>({})
  const [platformHost, setPlatformHost] = useState("")
  useEffect(() => {
    setPlatformHost(window.location.host)
  }, [])

  function clearField(key: FieldKey) {
    setFieldErrors((prev) => {
      if (!(key in prev)) return prev
      const next = { ...prev }
      delete next[key]
      return next
    })
  }

  async function submit() {
    setError(null)
    const errs: Partial<Record<FieldKey, string>> = {}
    if (!name.trim()) errs.name = "Name ist erforderlich."
    if (!SLUG_RE.test(slug))
      errs.slug = "Ungültiger Slug. Erlaubt: Kleinbuchstaben, Ziffern und Bindestriche (nicht am Anfang oder Ende)."
    setFieldErrors(errs)
    if (Object.keys(errs).length > 0) return

    setSaving(true)
    try {
      const res = await fetchAuth("/api/v1/platform/tenants", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
        body: JSON.stringify({ slug, name }),
      })
      if (!res.ok) {
        const info = await readApiError(res)
        if (info.code === "slug_taken") {
          setFieldErrors({ slug: "Dieser Slug ist bereits vergeben." })
        } else {
          setError(info.messageDe)
        }
        return
      }
      const t = (await res.json()) as { id: string }
      router.push(`/platform-admin/tenants/${t.id}`)
    } catch {
      setError("Erstellen fehlgeschlagen. Bitte versuche es erneut.")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="max-w-lg space-y-6">
      <h1 className="font-primary text-2xl font-semibold">Tenant erstellen</h1>
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
            <Input
              id="name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                clearField("name")
              }}
              placeholder="Acme Bistro"
              aria-invalid={!!fieldErrors.name}
            />
            {fieldErrors.name && <p className="text-destructive text-sm">{fieldErrors.name}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="slug">Slug</Label>
            <Input
              id="slug"
              value={slug}
              onChange={(e) => {
                setSlug(e.target.value.toLowerCase().trim())
                clearField("slug")
              }}
              placeholder="acme"
              maxLength={63}
              aria-invalid={!!fieldErrors.slug}
            />
            {fieldErrors.slug && <p className="text-destructive text-sm">{fieldErrors.slug}</p>}
            <p className="text-muted-foreground text-xs">
              Subdomain: {slug || "<slug>"}.{platformHost || "…"}
            </p>
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
