"use client"

import { AlertTriangle, CheckCircle2, CloudDownload, Loader2, WifiOff } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useOfflineReadiness } from "@/lib/station/use-offline-readiness"

export function OfflineReadiness() {
  const { state, online, prepare } = useOfflineReadiness()

  if (state === "unsupported" || state === "checking") return null

  if (state === "preparing") {
    return (
      <Row tone="muted">
        <Loader2 className="h-4 w-4 shrink-0 animate-spin" aria-hidden />
        <span>Wird für Offline-Betrieb vorbereitet…</span>
      </Row>
    )
  }

  if (state === "ready") {
    return (
      <Row tone="success" live>
        {online ? (
          <CheckCircle2 className="h-4 w-4 shrink-0" aria-hidden />
        ) : (
          <WifiOff className="h-4 w-4 shrink-0" aria-hidden />
        )}
        <span className="flex-1">{online ? "Bereit für Offline-Betrieb" : "Offline – bereit zum Scannen"}</span>
        {online && (
          <button
            type="button"
            onClick={prepare}
            className="text-success/80 hover:text-success shrink-0 text-xs underline underline-offset-2"
          >
            Erneut vorbereiten
          </button>
        )}
      </Row>
    )
  }

  if (state === "stale") {
    return (
      <Card
        icon={<AlertTriangle className="h-5 w-5" aria-hidden />}
        tone="warning"
        title="App wurde aktualisiert"
        body="Bereite den Offline-Betrieb erneut vor, solange du online bist."
      >
        <PrepareButton online={online} onClick={prepare} label="Erneut vorbereiten" />
      </Card>
    )
  }

  if (state === "error") {
    return (
      <Card
        icon={<AlertTriangle className="h-5 w-5" aria-hidden />}
        tone="warning"
        title="Vorbereitung fehlgeschlagen"
        body="Nicht alle Inhalte konnten zwischengespeichert werden. Bitte erneut versuchen, solange du online bist."
      >
        <PrepareButton online={online} onClick={prepare} label="Erneut versuchen" />
      </Card>
    )
  }

  // unprepared
  return (
    <Card
      icon={<CloudDownload className="h-5 w-5" aria-hidden />}
      tone="muted"
      title="Offline-Betrieb einrichten"
      body="Lade alle Inhalte einmalig herunter, damit das Scannen auch ohne Internet funktioniert."
    >
      <PrepareButton online={online} onClick={prepare} label="Für Offline-Betrieb vorbereiten" />
    </Card>
  )
}

function PrepareButton({ online, onClick, label }: { online: boolean; onClick: () => void; label: string }) {
  return (
    <div className="mt-4 grid gap-2">
      <Button type="button" onClick={onClick} disabled={!online} className="w-full">
        {label}
      </Button>
      {!online && (
        <p className="text-muted-foreground inline-flex items-center justify-center gap-1.5 text-xs">
          <WifiOff className="h-3.5 w-3.5" aria-hidden />
          Mit dem Internet verbinden, um die Vorbereitung abzuschließen.
        </p>
      )}
    </div>
  )
}

const TONE_ROW: Record<"muted" | "success", string> = {
  muted: "bg-card text-muted-foreground",
  success: "bg-success/10 text-success",
}

function Row({ tone, live, children }: { tone: "muted" | "success"; live?: boolean; children: React.ReactNode }) {
  return (
    <div
      role="status"
      aria-live={live ? "polite" : undefined}
      className={`${TONE_ROW[tone]} flex w-full items-center gap-2 rounded-2xl px-4 py-3 text-sm font-medium`}
    >
      {children}
    </div>
  )
}

const TONE_CARD: Record<"muted" | "warning", { wrap: string; icon: string }> = {
  muted: { wrap: "bg-card", icon: "bg-primary/10 text-primary" },
  warning: { wrap: "bg-destructive/5", icon: "bg-destructive/10 text-destructive" },
}

function Card({
  icon,
  tone,
  title,
  body,
  children,
}: {
  icon: React.ReactNode
  tone: "muted" | "warning"
  title: string
  body: string
  children?: React.ReactNode
}) {
  const styles = TONE_CARD[tone]
  return (
    <div role="status" className={`${styles.wrap} w-full rounded-2xl p-6`}>
      <div className={`${styles.icon} mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full`}>
        {icon}
      </div>
      <h2 className="font-primary text-center text-xl tracking-wide uppercase">{title}</h2>
      <p className="text-muted-foreground mt-1 text-center text-sm">{body}</p>
      {children}
    </div>
  )
}
