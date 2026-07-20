"use client"

import { useCallback, useEffect, useRef } from "react"
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

  // Reset on (re)open. The consumer auto-closes via a controlled prop change on a
  // hit, which never triggers Dialog.onOpenChange, so relying on that to clear the
  // lock stranded it at true and silently swallowed every subsequent scan.
  useEffect(() => {
    if (open) lockedRef.current = false
  }, [open])

  const handleResult = useCallback(
    (result: QrScanResult) => {
      if (lockedRef.current) return
      lockedRef.current = true
      onScan(result)
    },
    [onScan]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
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
