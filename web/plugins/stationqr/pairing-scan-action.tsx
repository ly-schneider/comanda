"use client"

import { Loader2, ScanLine } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import QrScanner, { type QrScannerControls, type QrScanResult } from "@/components/qr-scanner"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { getClientInfo } from "@/lib/client-info"
import { randomUrlSafe } from "@/lib/crypto"
import { setDeviceToken, setDeviceType } from "@/lib/device-auth"
import type { SlotComponentProps } from "@/plugins/slots"
import { enrollDevice, EnrollError } from "./api"
import { extractEnrollToken } from "./enroll-code"

function getDeviceKey(): string {
  const storageKey = "comanda.pairingDeviceKey.station"
  let k = localStorage.getItem(storageKey)
  if (!k) {
    k = `dev_${randomUrlSafe(24)}`
    localStorage.setItem(storageKey, k)
  }
  return k
}

async function performEnroll(token: string, name: string): Promise<void> {
  const info = await getClientInfo()
  const result = await enrollDevice({
    token,
    deviceKey: getDeviceKey(),
    name: name.trim() || undefined,
    model: info.model,
    os: info.os,
  })
  setDeviceToken(result.token)
  setDeviceType("station")
}

function enrollErrorMessage(e: unknown): string {
  if (e instanceof EnrollError) {
    if (e.code === "invalid_token") return "Der Code ist ungültig oder wurde erneuert. Bitte neuen QR-Code drucken."
    if (e.code === "enrollment_disabled") return "Dieser QR-Code ist zurzeit deaktiviert."
  }
  return "Anmeldung fehlgeschlagen. Prüfe die Verbindung und versuche es erneut."
}

function stripCodeFromURL() {
  window.history.replaceState(null, "", window.location.pathname)
}

export default function StationqrPairingScanAction({ ctx }: SlotComponentProps<"device.pairing.actions">) {
  const [urlCode, setUrlCode] = useState<string | null>(null)

  useEffect(() => {
    const code = new URLSearchParams(window.location.search).get("code")
    if (code) setUrlCode(code)
  }, [])

  if (ctx.deviceType !== "STATION") return null
  return (
    <>
      {urlCode && (
        <AutoEnrollDialog
          code={urlCode}
          onDismiss={() => {
            stripCodeFromURL()
            setUrlCode(null)
          }}
        />
      )}
      <ScanAction />
    </>
  )
}

function AutoEnrollDialog({ code, onDismiss }: { code: string; onDismiss: () => void }) {
  const [name, setName] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      await performEnroll(code, name)
      stripCodeFromURL()
      window.location.reload()
    } catch (e) {
      setError(enrollErrorMessage(e))
      setBusy(false)
    }
  }, [code, name])

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onDismiss()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Station anmelden</DialogTitle>
        </DialogHeader>

        <p className="text-muted-foreground text-sm">
          Dieses Gerät wird als Station registriert und erhält die zugewiesenen Produkte.
        </p>

        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}

        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor="stationqr-auto-name">Gerätename (optional)</Label>
            <Input
              id="stationqr-auto-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={20}
              placeholder="z. B. Grill 1"
              autoFocus
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onDismiss} disabled={busy}>
              Abbrechen
            </Button>
            <Button type="submit" disabled={busy}>
              {busy && <Loader2 className="mr-2 size-4 animate-spin" />}
              Anmelden
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ScanAction() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const scannerRef = useRef<QrScannerControls | null>(null)
  const lockedRef = useRef(false)

  const handleResult = useCallback(
    async ({ text }: QrScanResult) => {
      if (!text || lockedRef.current) return
      lockedRef.current = true
      try {
        scannerRef.current?.pause()
      } catch {}
      setBusy(true)
      setError(null)
      const token = extractEnrollToken(text)
      if (!token) {
        setError("Kein gültiger Enrollment-Code.")
        setBusy(false)
        lockedRef.current = false
        try {
          scannerRef.current?.resume()
        } catch {}
        return
      }
      try {
        await performEnroll(token, name)
        window.location.reload()
      } catch (e) {
        setError(enrollErrorMessage(e))
        setBusy(false)
        lockedRef.current = false
        try {
          scannerRef.current?.resume()
        } catch {}
      }
    },
    [name]
  )

  function close() {
    setOpen(false)
    setError(null)
    setBusy(false)
    lockedRef.current = false
  }

  return (
    <div className="mt-4 w-full max-w-md">
      <Button variant="outline" className="w-full" onClick={() => setOpen(true)}>
        <ScanLine className="mr-2 size-4" />
        Enrollment-Code scannen
      </Button>

      <Dialog open={open} onOpenChange={(o) => (o ? setOpen(true) : close())}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Station anmelden</DialogTitle>
          </DialogHeader>

          {error && (
            <div role="alert" className="text-destructive bg-destructive/10 rounded px-3 py-2 text-sm">
              {error}
            </div>
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="stationqr-name">Gerätename (optional)</Label>
            <Input
              id="stationqr-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={20}
              placeholder="z. B. Grill 1"
            />
          </div>

          <div className="relative w-full overflow-hidden rounded-2xl">
            <QrScanner
              fps={10}
              maxWidthRem={22}
              onResult={handleResult}
              onReady={(c) => {
                scannerRef.current = c
              }}
              onStartError={() => setError("Kamera konnte nicht gestartet werden.")}
              onStop={() => {
                scannerRef.current = null
              }}
            />
            {busy && (
              <div className="bg-background/70 absolute inset-0 flex items-center justify-center">
                <Loader2 className="text-muted-foreground size-6 animate-spin" aria-hidden />
              </div>
            )}
          </div>

          <p className="text-muted-foreground text-center text-xs">Halte den ausgehängten QR-Code in den Rahmen.</p>
        </DialogContent>
      </Dialog>
    </div>
  )
}
