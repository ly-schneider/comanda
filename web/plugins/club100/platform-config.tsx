"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { PlatformPluginConfigProps } from "@/plugins/slots"

export default function Club100PlatformConfig({ secretsConfiguredAt, onSave, saving }: PlatformPluginConfigProps) {
  const [apiKey, setApiKey] = useState("")
  const [groupId, setGroupId] = useState("")

  const placeholder = secretsConfiguredAt ? "Konfiguriert – zum Ändern neu eingeben" : "Nicht konfiguriert"

  async function handleSave() {
    await onSave({ secrets: { apiKey, groupId } })
    setApiKey("")
    setGroupId("")
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="club100-elvanto-key">Elvanto API Key</Label>
        <Input
          id="club100-elvanto-key"
          type="password"
          autoComplete="off"
          placeholder={placeholder}
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          disabled={saving}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="club100-elvanto-group">Elvanto Group ID</Label>
        <Input
          id="club100-elvanto-group"
          autoComplete="off"
          placeholder={placeholder}
          value={groupId}
          onChange={(e) => setGroupId(e.target.value)}
          disabled={saving}
        />
      </div>
      <Button size="sm" onClick={handleSave} disabled={saving || (!apiKey && !groupId)}>
        Elvanto-Zugang speichern
      </Button>
    </div>
  )
}
