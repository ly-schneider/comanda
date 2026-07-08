"use client"

import { useCallback, useRef } from "react"
import QrScanner, { type QrScanResult } from "@/components/qr-scanner"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"

interface OrderScannerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onScan: (result: QrScanResult) => void
}

// The Android WebView grants the camera; mounts only while open so it releases on close.
export function OrderScanner({ open, onOpenChange, onScan }: OrderScannerProps) {
  const lockedRef = useRef(false)

  const handleResult = useCallback(
    (result: QrScanResult) => {
      if (lockedRef.current) return
      lockedRef.current = true
      onScan(result)
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
        {open && <QrScanner fps={10} maxWidthRem={24} onResult={handleResult} />}
      </DialogContent>
    </Dialog>
  )
}
