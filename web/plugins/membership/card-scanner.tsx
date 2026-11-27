"use client"

import dynamic from "next/dynamic"
import { useCallback, useRef, useState } from "react"
import type { QrScanResult } from "@/components/qr-scanner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

// The camera (and its wasm decoder) is only loaded where a card is scanned.
const QrScanner = dynamic(() => import("@/components/qr-scanner"), { ssr: false })

interface CardScannerProps {
  onCode: (code: string) => void
  busy?: boolean
  manualLabel?: string
}

// Reads a membership card: the camera where there is one, typing where there
// isn't. What the code means is the provider's business, so whatever the card
// holds is passed on unchanged.
export function CardScanner({ onCode, busy = false, manualLabel = "Nummer eintippen" }: CardScannerProps) {
  const [cameraFailed, setCameraFailed] = useState(false)
  const [manual, setManual] = useState("")
  const lockedRef = useRef(false)

  const handleResult = useCallback(
    (result: QrScanResult) => {
      if (lockedRef.current || busy) return
      lockedRef.current = true
      onCode(result.text)
    },
    [busy, onCode]
  )

  const submitManual = useCallback(() => {
    const code = manual.trim()
    if (!code || busy) return
    onCode(code)
  }, [busy, manual, onCode])

  return (
    <div className="space-y-3">
      {!cameraFailed && (
        <QrScanner fps={10} maxWidthRem={22} onResult={handleResult} onStartError={() => setCameraFailed(true)} />
      )}
      {cameraFailed && (
        <p className="text-muted-foreground text-sm">Keine Kamera verfügbar. Gib die Nummer von der Karte ein.</p>
      )}
      <div className="flex gap-2">
        <Input
          value={manual}
          onChange={(e) => setManual(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitManual()
          }}
          placeholder={manualLabel}
          aria-label={manualLabel}
          autoComplete="off"
          disabled={busy}
        />
        <Button variant="outline" onClick={submitManual} disabled={busy || manual.trim() === ""}>
          Prüfen
        </Button>
      </div>
    </div>
  )
}
