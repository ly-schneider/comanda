"use client"

import Image from "next/image"
import { useEffect, useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { DEFAULT_FONT_KEY, DEFAULT_RADIUS_KEY, FONT_OPTIONS, RADIUS_OPTIONS } from "@/lib/branding-theme"
import { getCSRFToken } from "@/lib/csrf"

export type BrandingSettings = {
  displayName?: string
  logoUrl?: string | null
  faviconUrl?: string | null
  colorPrimary?: string
  colorSecondary?: string
  fontKey?: string
  radiusKey?: string
  contactEmail?: string | null
  contactPhone?: string | null
  legalName?: string | null
  legalAddress?: string | null
  currency?: string
}

const CURRENCY_OPTIONS = ["CHF", "EUR"] as const

const HEX_RE = /^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/

function isValidHex(v: string) {
  return HEX_RE.test(v)
}

// BrandingSettingsForm is the account-hub branding/legal editor; the tenant
// flows in via basePath (/api/v1/account/<tenant>/settings).
export function BrandingSettingsForm({ basePath, heading = "Branding" }: { basePath: string; heading?: string }) {
  const fetchAuth = useAuthorizedFetch()

  const [settings, setSettings] = useState<BrandingSettings>({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  const [logoUploading, setLogoUploading] = useState(false)
  const [faviconUploading, setFaviconUploading] = useState(false)
  const [logoDeleting, setLogoDeleting] = useState(false)
  const [faviconDeleting, setFaviconDeleting] = useState(false)
  const [uploadError, setUploadError] = useState<string | null>(null)

  const logoInputRef = useRef<HTMLInputElement>(null)
  const faviconInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    ;(async () => {
      const res = await fetchAuth(basePath)
      if (res.ok) {
        const data = (await res.json()) as BrandingSettings
        setSettings({
          displayName: data.displayName ?? "",
          logoUrl: data.logoUrl ?? null,
          faviconUrl: data.faviconUrl ?? null,
          colorPrimary: data.colorPrimary ?? "#0a0a0a",
          colorSecondary: data.colorSecondary ?? "#f5f5f5",
          fontKey: data.fontKey ?? DEFAULT_FONT_KEY,
          radiusKey: data.radiusKey ?? DEFAULT_RADIUS_KEY,
          contactEmail: data.contactEmail ?? "",
          contactPhone: data.contactPhone ?? "",
          legalName: data.legalName ?? "",
          legalAddress: data.legalAddress ?? "",
          currency: data.currency ?? "CHF",
        })
      }
    })()
  }, [fetchAuth, basePath])

  function field<K extends keyof BrandingSettings>(key: K) {
    return (value: BrandingSettings[K]) => setSettings((prev) => ({ ...prev, [key]: value }))
  }

  async function save() {
    setSaving(true)
    setError(null)
    setSuccess(false)
    try {
      const csrf = getCSRFToken()
      const body: Record<string, string | null | undefined> = {
        displayName: settings.displayName,
        colorPrimary: settings.colorPrimary,
        colorSecondary: settings.colorSecondary,
        fontKey: settings.fontKey,
        radiusKey: settings.radiusKey,
        contactEmail: settings.contactEmail || null,
        contactPhone: settings.contactPhone || null,
        legalName: settings.legalName || null,
        legalAddress: settings.legalAddress || null,
        currency: settings.currency,
      }

      const hexFields = ["colorPrimary", "colorSecondary"] as const
      for (const f of hexFields) {
        const v = body[f]
        if (v && !isValidHex(v)) {
          setError(`Ungültiger Hex-Wert für ${f}: ${v}`)
          setSaving(false)
          return
        }
      }

      const res = await fetchAuth(basePath, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { code?: string }
        throw new Error(
          j.code === "invalid_body"
            ? "Ungültige Eingabe: Bitte prüfe Farben (Hex) und Währung."
            : "Speichern fehlgeschlagen. Bitte versuche es erneut."
        )
      }
      setSuccess(true)
      setTimeout(() => setSuccess(false), 3000)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  async function uploadLogo(file: File, kind: "logo" | "favicon") {
    setUploadError(null)
    if (kind === "logo") setLogoUploading(true)
    else setFaviconUploading(true)
    try {
      const csrf = getCSRFToken()
      const form = new FormData()
      form.append("file", file)
      form.append("kind", kind)
      const res = await fetchAuth(`${basePath}/logo`, {
        method: "POST",
        headers: { "X-CSRF": csrf || "" },
        body: form,
      })
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { message?: string; detail?: string }
        throw new Error(j.message || j.detail || `Upload fehlgeschlagen (${res.status})`)
      }
      const data = (await res.json()) as { url: string }
      if (kind === "logo") setSettings((prev) => ({ ...prev, logoUrl: data.url }))
      else setSettings((prev) => ({ ...prev, faviconUrl: data.url }))
    } catch (e) {
      setUploadError(e instanceof Error ? e.message : "Upload fehlgeschlagen")
    } finally {
      if (kind === "logo") setLogoUploading(false)
      else setFaviconUploading(false)
    }
  }

  async function deleteLogo(kind: "logo" | "favicon") {
    setUploadError(null)
    if (kind === "logo") setLogoDeleting(true)
    else setFaviconDeleting(true)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`${basePath}/logo?kind=${kind}`, {
        method: "DELETE",
        headers: { "X-CSRF": csrf || "" },
      })
      if (!res.ok && res.status !== 204) {
        throw new Error(`Löschen fehlgeschlagen (${res.status})`)
      }
      if (kind === "logo") setSettings((prev) => ({ ...prev, logoUrl: null }))
      else setSettings((prev) => ({ ...prev, faviconUrl: null }))
    } catch (e) {
      setUploadError(e instanceof Error ? e.message : "Löschen fehlgeschlagen")
    } finally {
      if (kind === "logo") setLogoDeleting(false)
      else setFaviconDeleting(false)
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
      {success && (
        <div role="status" className="rounded bg-green-100 px-3 py-2 text-green-800">
          Gespeichert
        </div>
      )}

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Allgemein</CardTitle>
          <p className="text-muted-foreground text-sm">Name und Währung des Tenants.</p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          <div className="space-y-2">
            <Label htmlFor="displayName">Anzeigename</Label>
            <Input
              id="displayName"
              value={settings.displayName ?? ""}
              onChange={(e) => field("displayName")(e.target.value)}
              className="rounded-xl"
              placeholder="z. B. Comanda"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="currency">Währung</Label>
            <Select value={settings.currency ?? "CHF"} onValueChange={(v) => field("currency")(v)}>
              <SelectTrigger id="currency" className="max-w-28 rounded-xl">
                <SelectValue placeholder="CHF" />
              </SelectTrigger>
              <SelectContent>
                {CURRENCY_OPTIONS.map((c) => (
                  <SelectItem key={c} value={c}>
                    {c}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Logo &amp; Favicon</CardTitle>
          <p className="text-muted-foreground text-sm">PNG, JPEG, WebP oder ICO, max. 5 MB.</p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-6">
          {uploadError && (
            <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
              {uploadError}
            </div>
          )}
          <div className="space-y-3">
            <Label>Logo</Label>
            {settings.logoUrl && (
              <div className="flex items-center gap-3">
                <Image src={settings.logoUrl} alt="Logo Vorschau" width={48} height={48} className="rounded-lg" />
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-xl"
                  onClick={() => deleteLogo("logo")}
                  disabled={logoDeleting}
                >
                  {logoDeleting ? "Löschen…" : "Entfernen"}
                </Button>
              </div>
            )}
            <input
              ref={logoInputRef}
              type="file"
              accept="image/png,image/jpeg,image/webp,image/x-icon"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) uploadLogo(file, "logo")
                e.target.value = ""
              }}
            />
            <Button
              variant="outline"
              size="sm"
              className="rounded-xl"
              onClick={() => logoInputRef.current?.click()}
              disabled={logoUploading}
            >
              {logoUploading ? "Hochladen…" : "Logo hochladen"}
            </Button>
          </div>

          <div className="space-y-3">
            <Label>Favicon</Label>
            {settings.faviconUrl && (
              <div className="flex items-center gap-3">
                <Image src={settings.faviconUrl} alt="Favicon Vorschau" width={32} height={32} className="rounded" />
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-xl"
                  onClick={() => deleteLogo("favicon")}
                  disabled={faviconDeleting}
                >
                  {faviconDeleting ? "Löschen…" : "Entfernen"}
                </Button>
              </div>
            )}
            <input
              ref={faviconInputRef}
              type="file"
              accept="image/png,image/jpeg,image/webp,image/x-icon"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) uploadLogo(file, "favicon")
                e.target.value = ""
              }}
            />
            <Button
              variant="outline"
              size="sm"
              className="rounded-xl"
              onClick={() => faviconInputRef.current?.click()}
              disabled={faviconUploading}
            >
              {faviconUploading ? "Hochladen…" : "Favicon hochladen"}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Farben</CardTitle>
          <p className="text-muted-foreground text-sm">
            Akzentfarben über dem festen, hellen Grunddesign. Hex-Werte (z. B. #0a0a0a).
          </p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          {(
            [
              { key: "colorPrimary", label: "Primärfarbe" },
              { key: "colorSecondary", label: "Sekundärfarbe" },
            ] as const
          ).map(({ key, label }) => (
            <div key={key} className="flex items-center gap-3">
              <div className="flex-1 space-y-1">
                <Label htmlFor={key}>{label}</Label>
                <div className="flex items-center gap-2">
                  <input
                    type="color"
                    value={isValidHex(settings[key] ?? "") ? settings[key]! : "#000000"}
                    onChange={(e) => field(key)(e.target.value)}
                    className="h-9 w-10 cursor-pointer rounded border p-0.5"
                    aria-label={`${label} Farbwähler`}
                  />
                  <Input
                    id={key}
                    value={settings[key] ?? ""}
                    onChange={(e) => field(key)(e.target.value)}
                    className="max-w-32 rounded-xl font-mono text-sm"
                    placeholder="#000000"
                    maxLength={9}
                  />
                </div>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Darstellung</CardTitle>
          <p className="text-muted-foreground text-sm">Schriftart und Eckenstil.</p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-6">
          <div className="space-y-2">
            <Label htmlFor="fontKey">Schriftart</Label>
            <Select value={settings.fontKey ?? DEFAULT_FONT_KEY} onValueChange={(v) => field("fontKey")(v)}>
              <SelectTrigger id="fontKey" className="max-w-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FONT_OPTIONS.map((o) => (
                  <SelectItem key={o.key} value={o.key}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Eckenstil</Label>
            <div className="flex flex-wrap gap-2">
              {RADIUS_OPTIONS.map((o) => (
                <Button
                  key={o.key}
                  type="button"
                  variant={(settings.radiusKey ?? DEFAULT_RADIUS_KEY) === o.key ? "primary" : "outline"}
                  size="sm"
                  onClick={() => field("radiusKey")(o.key)}
                >
                  {o.label}
                </Button>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Kontakt</CardTitle>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          <div className="space-y-2">
            <Label htmlFor="contactEmail">E-Mail</Label>
            <Input
              id="contactEmail"
              type="email"
              value={settings.contactEmail ?? ""}
              onChange={(e) => field("contactEmail")(e.target.value)}
              className="rounded-xl"
              placeholder="info@example.ch"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="contactPhone">Telefon</Label>
            <Input
              id="contactPhone"
              type="tel"
              value={settings.contactPhone ?? ""}
              onChange={(e) => field("contactPhone")(e.target.value)}
              className="rounded-xl"
              placeholder="+41 00 000 00 00"
            />
          </div>
        </CardContent>
      </Card>

      <Card className="rounded-2xl">
        <CardHeader>
          <CardTitle>Rechtliches</CardTitle>
          <p className="text-muted-foreground text-sm">
            Wird in Datenschutzerklärung und Nutzungsbedingungen angezeigt.
          </p>
        </CardHeader>
        <CardContent className="max-w-lg space-y-4">
          <div className="space-y-2">
            <Label htmlFor="legalName">Firmenname</Label>
            <Input
              id="legalName"
              value={settings.legalName ?? ""}
              onChange={(e) => field("legalName")(e.target.value)}
              className="rounded-xl"
              placeholder="Muster GmbH"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="legalAddress">Adresse</Label>
            <Input
              id="legalAddress"
              value={settings.legalAddress ?? ""}
              onChange={(e) => field("legalAddress")(e.target.value)}
              className="rounded-xl"
              placeholder="Musterstrasse 1, 3000 Bern"
            />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end pb-6">
        <Button onClick={save} disabled={saving} className="h-11 min-w-32 rounded-xl">
          {saving ? "Speichern…" : "Speichern"}
        </Button>
      </div>
    </div>
  )
}
