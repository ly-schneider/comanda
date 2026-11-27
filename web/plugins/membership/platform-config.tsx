"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { PlatformPluginConfigProps } from "@/plugins/slots"

export default function MembershipPlatformConfig({ secretsConfiguredAt, onSave, saving }: PlatformPluginConfigProps) {
  const [baseUrl, setBaseUrl] = useState("")
  const [apiKey, setApiKey] = useState("")

  const placeholder = secretsConfiguredAt ? "Konfiguriert – zum Ändern neu eingeben" : "Nicht konfiguriert"

  // Secrets are stored as one map, so both fields are written together.
  async function handleSave() {
    await onSave({ secrets: { baseUrl: baseUrl.trim(), apiKey: apiKey.trim() } })
    setBaseUrl("")
    setApiKey("")
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="membership-base-url">Basis-URL des Anbieters</Label>
        <Input
          id="membership-base-url"
          autoComplete="off"
          inputMode="url"
          placeholder={placeholder}
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          disabled={saving}
        />
        <p className="text-muted-foreground text-xs">
          Ohne <code>/lookup</code>, <code>/redeem</code> oder <code>/void</code> – die hängt Comanda an. Nur https.
        </p>
      </div>
      <div className="space-y-1">
        <Label htmlFor="membership-api-key">Schlüssel</Label>
        <Input
          id="membership-api-key"
          type="password"
          autoComplete="off"
          placeholder={placeholder}
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          disabled={saving}
        />
      </div>
      <Button size="sm" onClick={handleSave} disabled={saving || !baseUrl.trim() || !apiKey.trim()}>
        Zugang speichern
      </Button>
    </div>
  )
}
