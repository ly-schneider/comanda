"use client"

import { Ban, ShieldCheck, Trash2 } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { type BlockState, isValidBlockId } from "@/lib/station/block-list"

function formatSyncAge(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  if (s < 60) return `${s} Sek.`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} Min.`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} Std.`
  return `${Math.floor(h / 24)} Tg.`
}

export function syncedLabel(syncedAt: number | null, now: number): string {
  if (syncedAt == null) return "Noch nicht synchronisiert"
  return `Zuletzt synchronisiert vor ${formatSyncAge(now - syncedAt)}`
}

export function StationBlockList({
  blockState,
  voucherEnabled,
  onBlock,
  onUnblock,
}: {
  blockState: BlockState
  voucherEnabled: boolean
  onBlock: (id: string) => Promise<void>
  onUnblock: (id: string) => Promise<void>
}) {
  const [entry, setEntry] = useState("")
  const [error, setError] = useState<string | null>(null)

  const manual = blockState.manual
  const serverCount = blockState.server.ids.length

  const submit = async () => {
    // Case-sensitive nanoids: trim whitespace, keep the case.
    const id = entry.trim()
    if (!isValidBlockId(id)) {
      setError("Ungültiger Code")
      return
    }
    setError(null)
    setEntry("")
    await onBlock(id)
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" aria-label="Sperrliste" className="relative px-2">
          <Ban className="size-4" aria-hidden />
          {manual.length > 0 && (
            <span
              aria-hidden
              className="bg-destructive text-primary-foreground absolute -top-1 -right-1 flex size-4 items-center justify-center rounded-full text-[10px] font-semibold tabular-nums"
            >
              {manual.length}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <div className="grid gap-3">
          <div className="grid gap-1">
            <div className="flex items-center gap-2 text-sm font-medium">
              <Ban className="size-4 shrink-0" aria-hidden />
              <span>Codes sperren</span>
            </div>
            {voucherEnabled && (
              <p className="text-muted-foreground inline-flex items-center gap-1.5 text-xs">
                <ShieldCheck className="size-3.5 shrink-0" aria-hidden />
                {syncedLabel(blockState.server.syncedAt, Date.now())}
                {serverCount > 0 && ` · ${serverCount} vom Server`}
              </p>
            )}
          </div>

          <form
            className="grid gap-1.5"
            onSubmit={(e) => {
              e.preventDefault()
              void submit()
            }}
          >
            <div className="flex items-center gap-2">
              <Input
                value={entry}
                onChange={(e) => {
                  setEntry(e.target.value)
                  if (error) setError(null)
                }}
                placeholder="Code eingeben"
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
                aria-label="Code zum Sperren"
                className="h-9 font-mono text-sm"
              />
              <Button type="submit" size="sm" className="h-9 shrink-0" disabled={entry.trim().length === 0}>
                Sperren
              </Button>
            </div>
            {error && <p className="text-destructive text-xs">{error}</p>}
          </form>

          {manual.length > 0 ? (
            <div className="grid gap-1">
              <p className="text-muted-foreground font-primary text-[11px] tracking-[0.08em] uppercase">
                Manuell gesperrt
              </p>
              <ul className="grid gap-1">
                {manual.map((id) => (
                  <li key={id} className="flex items-center justify-between gap-2">
                    <span className="truncate font-mono text-sm">{id}</span>
                    <button
                      type="button"
                      onClick={() => void onUnblock(id)}
                      aria-label={`Sperre für ${id} aufheben`}
                      className="text-muted-foreground hover:bg-foreground/5 hover:text-foreground inline-flex size-7 shrink-0 items-center justify-center rounded-full transition-colors"
                    >
                      <Trash2 className="size-4" aria-hidden />
                    </button>
                  </li>
                ))}
              </ul>
              <p className="text-muted-foreground text-xs">
                Manuelle Sperren werden bei der nächsten Synchronisierung durch den Server ersetzt.
              </p>
            </div>
          ) : (
            <p className="text-muted-foreground text-xs">
              Sperre einen Abhol- oder Gutschein-Code beim Scan oder gib ihn hier ein.
            </p>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
