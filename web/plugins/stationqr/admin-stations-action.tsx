"use client"

import { Loader2, Printer, QrCode, RefreshCw } from "lucide-react"
import QRCode from "qrcode"
import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import type { SlotComponentProps } from "@/plugins/slots"
import { getCode, rotateCode, setCodeDisabled, type StationqrCode } from "./api"

function printQr(dataUrl: string, onBlocked: () => void) {
  const win = window.open("", "_blank", "width=420,height=560")
  if (!win) {
    onBlocked()
    return
  }
  const doc = win.document
  doc.title = "Station-Registrierung"
  const style = doc.createElement("style")
  style.textContent =
    "body{font-family:system-ui,sans-serif;text-align:center;margin:0;padding:32px;color:#111}h1{font-size:20px;margin:0 0 8px}p{color:#555;font-size:14px;margin:4px 0 24px}img{width:340px;height:340px}"
  doc.head.appendChild(style)
  const h1 = doc.createElement("h1")
  h1.textContent = "Station anmelden"
  const p = doc.createElement("p")
  p.textContent = "Auf einem neuen Stationsgerät scannen."
  const img = doc.createElement("img")
  img.src = dataUrl
  img.alt = "Enrollment QR-Code"
  img.addEventListener("load", () => {
    win.focus()
    win.print()
  })
  doc.body.appendChild(h1)
  doc.body.appendChild(img)
  doc.body.appendChild(p)
}

export default function StationqrAdminStationsAction({ ctx }: SlotComponentProps<"admin.stations.actions">) {
  const { token } = ctx
  const [code, setCode] = useState<StationqrCode | null>(null)
  const [qrDataUrl, setQrDataUrl] = useState<string>("")
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    ;(async () => {
      try {
        setCode(await getCode(token))
      } catch (e) {
        setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
      } finally {
        setLoading(false)
      }
    })()
  }, [token])

  useEffect(() => {
    if (!code?.token) {
      setQrDataUrl("")
      return
    }
    QRCode.toDataURL(code.token, { width: 240, margin: 1 })
      .then(setQrDataUrl)
      .catch(() => setQrDataUrl(""))
  }, [code?.token])

  const rotate = useCallback(async () => {
    if (code?.configured && !window.confirm("Neuen Code generieren? Der bisherige Ausdruck wird ungültig.")) return
    setBusy(true)
    setError(null)
    try {
      setCode(await rotateCode(token))
    } catch (e) {
      setError(e instanceof Error ? e.message : "Erstellen fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }, [token, code?.configured])

  const toggle = useCallback(
    async (active: boolean) => {
      setBusy(true)
      setError(null)
      try {
        setCode(await setCodeDisabled(token, !active))
      } catch (e) {
        setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
      } finally {
        setBusy(false)
      }
    },
    [token]
  )

  return (
    <Card className="mb-6 rounded-2xl">
      <CardHeader>
        <CardTitle>Schnell-Registrierung per QR</CardTitle>
        <p className="text-muted-foreground text-sm">
          Drucke diesen QR-Code aus und hänge ihn auf. Neue Stationen scannen ihn, um sich ohne Kopplungscode anzumelden
          — beliebig oft.
        </p>
      </CardHeader>
      <CardContent>
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 mb-4 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}

        {loading ? (
          <Loader2 className="text-muted-foreground size-5 animate-spin" aria-hidden />
        ) : !code?.configured ? (
          <Button onClick={rotate} disabled={busy}>
            {busy ? <Loader2 className="mr-2 size-4 animate-spin" /> : <QrCode className="mr-2 size-4" />}
            QR-Code erstellen
          </Button>
        ) : (
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
            <div className={`rounded-xl border bg-white p-3 ${code.disabled ? "opacity-40" : ""}`}>
              {qrDataUrl ? (
                // eslint-disable-next-line @next/next/no-img-element -- local data URL, not a remote asset to optimize
                <img src={qrDataUrl} alt="Enrollment QR-Code" width={180} height={180} />
              ) : (
                <div className="flex size-[180px] items-center justify-center">
                  <Loader2 className="text-muted-foreground size-5 animate-spin" aria-hidden />
                </div>
              )}
            </div>
            <div className="flex flex-col items-start gap-3">
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  onClick={() => printQr(qrDataUrl, () => setError("Pop-up blockiert. Bitte Pop-ups erlauben."))}
                  disabled={!qrDataUrl}
                >
                  <Printer className="mr-2 size-4" />
                  Drucken
                </Button>
                <Button variant="outline" onClick={rotate} disabled={busy}>
                  <RefreshCw className="mr-2 size-4" />
                  Neu generieren
                </Button>
              </div>
              <div className="flex items-center gap-2">
                <Switch id="stationqr-active" checked={!code.disabled} disabled={busy} onCheckedChange={toggle} />
                <Label htmlFor="stationqr-active" className="text-sm">
                  Registrierung aktiv
                </Label>
              </div>
              {code.disabled && (
                <p className="text-muted-foreground text-xs">
                  Deaktiviert — Stationen können sich aktuell nicht anmelden.
                </p>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
