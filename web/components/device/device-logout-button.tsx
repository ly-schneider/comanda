"use client"

import { Loader2, LogOut } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
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
import { type DeviceType, getDeviceToken } from "@/lib/device-auth"
import { logoutDevice, unsyncedWorkCount } from "@/lib/device-logout"
import { cn } from "@/lib/utils"

interface DeviceLogoutButtonProps {
  deviceType: DeviceType
  className?: string
}

const LABELS: Record<DeviceType, { title: string; unit: string }> = {
  pos: { title: "POS abmelden", unit: "Bestellung" },
  station: { title: "Station abmelden", unit: "Einlösung" },
}

export function DeviceLogoutButton({ deviceType, className }: DeviceLogoutButtonProps) {
  const [mounted, setMounted] = useState(false)
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(0)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (loading) return
      setOpen(next)
      if (next) {
        void unsyncedWorkCount(deviceType)
          .then(setPending)
          .catch(() => setPending(0))
      }
    },
    [deviceType, loading]
  )

  const handleConfirm = useCallback(async () => {
    setLoading(true)
    await logoutDevice(deviceType)
  }, [deviceType])

  // `mounted` gate avoids an SSR/client mismatch on the token read.
  if (!mounted || !getDeviceToken()) return null

  const { title, unit } = LABELS[deviceType]

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <Button
        variant="outline"
        size="icon"
        aria-label={title}
        className={cn("rounded-[11px] border-0", className)}
        onClick={() => handleOpenChange(true)}
      >
        <LogOut className="size-4" />
      </Button>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>
            Dieses Gerät wird abgemeldet und muss neu registriert werden.
            {pending > 0 && (
              <>
                {" "}
                {pending} nicht synchronisierte {pending === 1 ? unit : `${unit}en`} gehen dabei verloren.
              </>
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={loading}>Abbrechen</AlertDialogCancel>
          <AlertDialogAction
            onClick={(e) => {
              e.preventDefault()
              void handleConfirm()
            }}
            disabled={loading}
          >
            {loading ? (
              <>
                <Loader2 className="size-4 animate-spin" aria-hidden />
                Wird abgemeldet…
              </>
            ) : (
              "Abmelden"
            )}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
