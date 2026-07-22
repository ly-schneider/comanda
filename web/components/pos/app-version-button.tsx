"use client"

import { Info, RefreshCw } from "lucide-react"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"

type Bridge = {
  getAppVersion?: () => string
  checkForUpdate?: () => void
}

function bridge(): Bridge | undefined {
  try {
    return (globalThis as unknown as { PosBridge?: Bridge }).PosBridge
  } catch {
    return undefined
  }
}

export function AppVersionButton() {
  const [open, setOpen] = useState(false)
  const [version, setVersion] = useState<{ name: string; code: number } | null>(null)

  useEffect(() => {
    const b = bridge()
    if (!b?.getAppVersion) return
    try {
      const parsed = JSON.parse(b.getAppVersion() || "{}") as { versionName?: string; versionCode?: number }
      if (parsed.versionName) setVersion({ name: parsed.versionName, code: parsed.versionCode ?? 0 })
    } catch {}
  }, [])

  if (!version) return null

  return (
    <>
      <Button
        variant="outline"
        size="icon"
        aria-label="App-Version"
        className="rounded-[11px] border-0"
        onClick={() => setOpen(true)}
      >
        <Info className="size-4" />
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>App-Version</DialogTitle>
          </DialogHeader>
          <div className="space-y-1">
            <div className="text-2xl font-semibold">Version {version.name}</div>
            <div className="text-muted-foreground text-sm">Build {version.code}</div>
          </div>
          <DialogFooter className="sm:justify-between">
            <Button variant="outline" onClick={() => bridge()?.checkForUpdate?.()} disabled={!bridge()?.checkForUpdate}>
              <RefreshCw className="mr-1 size-4" />
              Nach Updates suchen
            </Button>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Schliessen
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
