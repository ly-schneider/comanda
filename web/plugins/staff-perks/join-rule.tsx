import type { ReactNode } from "react"

// The connector between two product groups: a hairline running to both edges
// with the UND/ODER control centred on it. Shared so the read-only campaign
// detail renders the same shape the creation form edits.
export function JoinRule({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-center gap-2 py-2">
      <span className="bg-border/70 h-px flex-1" aria-hidden />
      <div className="bg-muted/60 border-border/60 inline-flex items-center gap-0.5 rounded-lg border p-[3px]">
        {children}
      </div>
      <span className="bg-border/70 h-px flex-1" aria-hidden />
    </div>
  )
}

export const joinPillClass =
  "rounded-md px-2.5 py-1 text-[11px] font-medium tracking-wide transition-[color,background-color,box-shadow]"

export const joinPillActiveClass = "bg-background text-foreground shadow-sm"
