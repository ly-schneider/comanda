"use client"

import { useEffect, useState } from "react"
import { PairDeviceCard } from "@/components/admin/pair-device-card"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type PosDevice = {
  id: string
  name: string
  model?: string
  os?: string
  token: string
  approved: boolean
  status: string
  approvedAt?: string
  createdAt?: string
}

export default function AdminPOSPage() {
  const fetchAuth = useAuthorizedFetch()
  const [devices, setDevices] = useState<PosDevice[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmRemove, setConfirmRemove] = useState<{ id: string; name: string } | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const r = await fetchAuth(`/api/v1/pos/devices`)
      const j = (await r.json()) as { items: PosDevice[] }
      setDevices(j.items || [])
    } catch {
      setError("Laden fehlgeschlagen")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const sortedDevices = [...devices].sort(
    (a, b) =>
      statusOrder(a.status) - statusOrder(b.status) ||
      (new Date(b.approvedAt ?? b.createdAt ?? "").getTime() || 0) -
        (new Date(a.approvedAt ?? a.createdAt ?? "").getTime() || 0)
  )

  async function removeDevice(id: string) {
    setConfirmRemove(null)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`/api/v1/devices/${id}`, {
        method: "DELETE",
        headers: { "X-CSRF": csrf || "" },
      })
      if (!res.ok && res.status !== 204) {
        const j = (await res.json().catch(() => ({}))) as { detail?: string; message?: string }
        throw new Error(j.detail || j.message || `Error ${res.status}`)
      }
      setDevices((prev) => prev.filter((d) => d.id !== id))
    } catch {
      setError("Entfernen fehlgeschlagen")
    }
  }

  return (
    <div className="pt-4">
      <h1 className="font-primary mb-4 text-2xl font-semibold">POS-Geräte</h1>

      <div className="mb-6">
        <PairDeviceCard onPaired={load} />
      </div>

      {error && (
        <div role="alert" className="text-destructive bg-destructive/10 mb-3 rounded px-3 py-2">
          {error}
        </div>
      )}
      {loading && <div className="text-muted-foreground">Lädt…</div>}

      <h2 className="mt-2 mb-2 text-lg font-semibold">Übersicht</h2>
      {!loading && sortedDevices.length === 0 && (
        <div className="text-muted-foreground">Keine POS-Geräte vorhanden.</div>
      )}
      <div className="space-y-3">
        {sortedDevices.map((d) => (
          <div key={d.id} className="bg-card border-border rounded-[11px] border p-3">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0 flex-1">
                <div className="truncate font-semibold">{d.name}</div>
                <div className="text-muted-foreground truncate text-sm">
                  {d.model || "Unbekanntes Modell"} • {d.os || "Unbekanntes OS"}
                </div>
                <div className="text-muted-foreground truncate text-sm">{maskKey(d.token)}</div>
                <div className="text-muted-foreground flex flex-wrap gap-2 text-xs">
                  {d.createdAt && <span>Erstellt: {new Date(d.createdAt).toLocaleString()}</span>}
                  {d.approvedAt && d.status === "approved" && (
                    <span>• Freigegeben: {new Date(d.approvedAt).toLocaleString()}</span>
                  )}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <StatusBadge status={d.status} />
                <Button
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:text-destructive"
                  onClick={() => setConfirmRemove({ id: d.id, name: d.name || d.id })}
                >
                  Entfernen
                </Button>
              </div>
            </div>
          </div>
        ))}
      </div>

      <AlertDialog
        open={!!confirmRemove}
        onOpenChange={(open) => {
          if (!open) setConfirmRemove(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Gerät entfernen</AlertDialogTitle>
            <AlertDialogDescription>
              Möchtest du &quot;{confirmRemove?.name}&quot; wirklich entfernen? Das Gerät verliert damit den Zugriff und
              muss erneut registriert werden.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (confirmRemove) void removeDevice(confirmRemove.id)
              }}
            >
              Entfernen
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const label =
    status === "approved"
      ? "Angenommen"
      : status === "rejected"
        ? "Abgelehnt"
        : status === "revoked"
          ? "Gesperrt"
          : "Ausstehend"
  const tone =
    status === "approved"
      ? "bg-emerald-100 text-emerald-800"
      : status === "rejected"
        ? "bg-destructive/10 text-destructive"
        : status === "revoked"
          ? "bg-slate-200 text-slate-700"
          : "bg-amber-100 text-amber-800"
  return <span className={`inline-flex h-8 items-center rounded-md px-2.5 text-xs font-medium ${tone}`}>{label}</span>
}

function statusOrder(status: string) {
  if (status === "pending") return 0
  if (status === "approved") return 1
  if (status === "revoked") return 2
  return 2
}

function maskKey(k: string) {
  if (!k) return ""
  if (k.length <= 6) return "•••"
  return `${k.slice(0, 3)}•••${k.slice(-3)}`
}
