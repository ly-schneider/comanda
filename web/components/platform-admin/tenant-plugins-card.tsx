"use client"

import { useCallback, useEffect, useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Switch } from "@/components/ui/switch"
import { getCSRFToken } from "@/lib/csrf"
import { getPlatformConfigComponent } from "@/plugins/registry"
import type { TenantPluginList, TenantPluginState, TenantPluginUpdate } from "@/types/plugin"

type FetchAuth = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

const UPDATE_ERRORS: Record<string, string> = {
  unknown_plugin: "Unbekanntes Plugin.",
}

function PluginRow({
  tenantId,
  plugin,
  saving,
  onSave,
}: {
  tenantId: string
  plugin: TenantPluginState
  saving: boolean
  onSave: (patch: TenantPluginUpdate) => Promise<void>
}) {
  const ConfigForm = getPlatformConfigComponent(plugin.key)

  return (
    <div className="border-border space-y-3 rounded-[var(--radius-control)] border p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="font-medium">{plugin.name}</div>
          <div className="text-muted-foreground text-xs">
            {plugin.secretsConfiguredAt ? "Konfiguriert" : "Nicht konfiguriert"}
          </div>
        </div>
        <Switch
          checked={plugin.enabled}
          disabled={saving}
          onCheckedChange={(checked) => onSave({ enabled: checked }).catch(() => {})}
        />
      </div>
      {ConfigForm && (
        <ConfigForm
          tenantId={tenantId}
          pluginKey={plugin.key}
          config={plugin.config}
          secretsConfiguredAt={plugin.secretsConfiguredAt ?? null}
          onSave={onSave}
          saving={saving}
        />
      )}
    </div>
  )
}

export function TenantPluginsCard({ tenantId, fetchAuth }: { tenantId: string; fetchAuth: FetchAuth }) {
  const [plugins, setPlugins] = useState<TenantPluginState[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [savingKey, setSavingKey] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const res = await fetchAuth(`/api/v1/platform/tenants/${tenantId}/plugins`)
      if (!res.ok) throw new Error(`Error ${res.status}`)
      const data = (await res.json()) as TenantPluginList
      setPlugins(data.items ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
    } finally {
      setLoading(false)
    }
  }, [fetchAuth, tenantId])

  useEffect(() => {
    load()
  }, [load])

  const save = useCallback(
    async (key: string, patch: TenantPluginUpdate) => {
      setSavingKey(key)
      setError(null)
      try {
        const res = await fetchAuth(`/api/v1/platform/tenants/${tenantId}/plugins/${key}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
          body: JSON.stringify(patch),
        })
        if (!res.ok) {
          if (res.status === 401 || res.status === 403) throw new Error("Keine Berechtigung für diese Aktion.")
          const body = (await res.json().catch(() => ({}))) as { code?: string }
          throw new Error(UPDATE_ERRORS[body.code ?? ""] ?? "Speichern fehlgeschlagen.")
        }
        const updated = (await res.json()) as TenantPluginState
        setPlugins((prev) => prev.map((p) => (p.key === updated.key ? updated : p)))
      } catch (e) {
        setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
        throw e
      } finally {
        setSavingKey(null)
      }
    },
    [fetchAuth, tenantId]
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>Plugins</CardTitle>
        <p className="text-muted-foreground text-sm">Aktiviere und konfiguriere Plugins für diesen Tenant.</p>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <div
            role="alert"
            className="text-destructive bg-destructive/10 rounded-[var(--radius-control)] px-3 py-2 text-sm"
          >
            {error}
          </div>
        )}
        {loading ? (
          <p className="text-muted-foreground text-sm">Laden…</p>
        ) : plugins.length === 0 ? (
          <p className="text-muted-foreground text-sm">Keine Plugins registriert.</p>
        ) : (
          plugins.map((plugin) => (
            <PluginRow
              key={plugin.key}
              tenantId={tenantId}
              plugin={plugin}
              saving={savingKey === plugin.key}
              onSave={(patch) => save(plugin.key, patch)}
            />
          ))
        )}
      </CardContent>
    </Card>
  )
}
