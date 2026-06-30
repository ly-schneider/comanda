"use client"

import { useCallback, useRef } from "react"
import Html5QrcodeScannerPlugin from "@/components/html5-qrcode-scanner-plugin"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"

interface OrderScannerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onScan: (raw: string) => void
}

// Reuses the station scanner plugin (back camera, Android WebView grants the
// camera automatically). Mounts only while open so the camera is released on
// close; locks after the first decode so a single QR fires once.
export function OrderScanner({ open, onOpenChange, onScan }: OrderScannerProps) {
  const lockedRef = useRef(false)

  const handleDecoded = useCallback(
    (decoded: string) => {
      if (lockedRef.current) return
      lockedRef.current = true
      onScan(decoded)
    },
    [onScan]
  )

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) lockedRef.current = false
        onOpenChange(v)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Bestellung scannen</DialogTitle>
          <DialogDescription>Halte den Abhol-QR der Bestellung vor die Kamera.</DialogDescription>
        </DialogHeader>
        {open && <Html5QrcodeScannerPlugin qrbox={240} fps={10} qrCodeSuccessCallback={handleDecoded} />}
      </DialogContent>
    </Dialog>
  )
}
