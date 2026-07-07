"use client"

import { AlertTriangle, CheckCircle2, CloudDownload, Loader2, RefreshCw, Wifi, WifiOff } from "lucide-react"
import type { ReactNode } from "react"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { type ReadinessState, useOfflineReadiness } from "@/lib/station/use-offline-readiness"

export function OfflineReadiness() {
  const { state, online, prepare } = useOfflineReadiness()

  if (state === "unsupported" || state === "checking") return null

  // The operator doesn't need a permanent banner for the healthy states — only a
  // subtle nudge when the offline shell still needs setting up or re-running.
  const attention = state === "unprepared" || state === "stale" || state === "error"

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" aria-label="Offline-Status" className="relative px-2">
          <StatusIcon state={state} online={online} />
          {attention && (
            <span
              aria-hidden
              className={`absolute -top-0.5 -right-0.5 size-2 rounded-full ${
                state === "error" ? "bg-destructive" : "bg-primary"
              }`}
            />
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72">
        <Detail state={state} online={online} prepare={prepare} />
      </PopoverContent>
    </Popover>
  )
}

function StatusIcon({ state, online }: { state: ReadinessState; online: boolean }) {
  if (state === "preparing") return <Loader2 className="size-4 animate-spin" aria-hidden />
  if (state === "error") return <AlertTriangle className="text-destructive size-4" aria-hidden />
  if (state === "unprepared" || state === "stale") return <CloudDownload className="size-4" aria-hidden />
  return online ? (
    <Wifi className="text-success size-4" aria-hidden />
  ) : (
    <WifiOff className="text-muted-foreground size-4" aria-hidden />
  )
}

function Detail({ state, online, prepare }: { state: ReadinessState; online: boolean; prepare: () => void }) {
  if (state === "preparing") {
    return (
      <Line
        icon={<Loader2 className="size-5 animate-spin" aria-hidden />}
        title="Wird für Offline-Betrieb vorbereitet…"
      />
    )
  }

  if (state === "ready") {
    return (
      <div className="grid gap-3">
        <Line
          icon={
            online ? (
              <CheckCircle2 className="text-success size-5" aria-hidden />
            ) : (
              <WifiOff className="size-5" aria-hidden />
            )
          }
          title={online ? "Bereit für Offline-Betrieb" : "Offline – bereit zum Scannen"}
          body={
            online
              ? "Alle Inhalte sind gespeichert. Das Scannen funktioniert auch ohne Internet."
              : "Keine Internetverbindung. Gespeicherte Inhalte werden verwendet."
          }
        />
        {online && (
          <Button type="button" variant="outline" size="sm" onClick={prepare} className="w-full">
            <RefreshCw className="size-4" aria-hidden />
            Erneut vorbereiten
          </Button>
        )}
      </div>
    )
  }

  if (state === "stale") {
    return (
      <Prepare
        icon={<AlertTriangle className="size-5" aria-hidden />}
        title="App wurde aktualisiert"
        body="Bereite den Offline-Betrieb erneut vor, solange du online bist."
        online={online}
        onClick={prepare}
        label="Erneut vorbereiten"
      />
    )
  }

  if (state === "error") {
    return (
      <Prepare
        icon={<AlertTriangle className="text-destructive size-5" aria-hidden />}
        title="Vorbereitung fehlgeschlagen"
        body="Nicht alle Inhalte konnten zwischengespeichert werden. Bitte erneut versuchen, solange du online bist."
        online={online}
        onClick={prepare}
        label="Erneut versuchen"
      />
    )
  }

  return (
    <Prepare
      icon={<CloudDownload className="size-5" aria-hidden />}
      title="Offline-Betrieb einrichten"
      body="Lade alle Inhalte einmalig herunter, damit das Scannen auch ohne Internet funktioniert."
      online={online}
      onClick={prepare}
      label="Für Offline-Betrieb vorbereiten"
    />
  )
}

function Line({ icon, title, body }: { icon: ReactNode; title: string; body?: string }) {
  return (
    <div className="grid gap-1.5">
      <div className="flex items-center gap-2 text-sm font-medium">
        <span className="shrink-0">{icon}</span>
        <span>{title}</span>
      </div>
      {body && <p className="text-muted-foreground text-sm">{body}</p>}
    </div>
  )
}

function Prepare({
  icon,
  title,
  body,
  online,
  onClick,
  label,
}: {
  icon: ReactNode
  title: string
  body: string
  online: boolean
  onClick: () => void
  label: string
}) {
  return (
    <div className="grid gap-3">
      <Line icon={icon} title={title} body={body} />
      <Button type="button" onClick={onClick} disabled={!online} size="sm" className="w-full">
        {label}
      </Button>
      {!online && (
        <p className="text-muted-foreground inline-flex items-center justify-center gap-1.5 text-xs">
          <WifiOff className="size-3.5" aria-hidden />
          Mit dem Internet verbinden, um die Vorbereitung abzuschliessen.
        </p>
      )}
    </div>
  )
}
