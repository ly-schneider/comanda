"use client"

import { Button } from "@/components/ui/button"
import { childrenOf, topLevelCategories } from "@/lib/categories"

export type FilterCategory = { id: string; name: string; position: number; parentId?: string | null }

function Chip({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <Button
      className={`border-border flex h-10 shrink-0 items-center rounded-[10px] border px-3 text-sm ${
        active
          ? "bg-primary text-primary-foreground hover:bg-primary-hover"
          : "text-muted-foreground hover:bg-card hover:text-foreground bg-transparent"
      }`}
      onClick={onClick}
      aria-pressed={active}
    >
      {label}
    </Button>
  )
}

function ChipRow({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex [scrollbar-width:none] gap-2 overflow-x-auto pb-1 [-ms-overflow-style:none] sm:flex-wrap sm:overflow-x-visible sm:pb-0 [&::-webkit-scrollbar]:hidden">
      {children}
    </div>
  )
}

export function CategoryFilter({
  categories,
  active,
  onChange,
  activeSub = "all",
  onSubChange,
}: {
  categories: FilterCategory[]
  active: string
  onChange: (value: string) => void
  activeSub?: string
  onSubChange?: (value: string) => void
}) {
  const tops = topLevelCategories(categories)
  const subs = active !== "all" ? childrenOf(categories, active) : []
  const showSubs = onSubChange != null && subs.length > 0

  return (
    <div className="space-y-2">
      <ChipRow>
        <Chip label="Alles" active={active === "all"} onClick={() => onChange("all")} />
        {tops.map((c) => (
          <Chip key={c.id} label={c.name} active={active === c.id} onClick={() => onChange(c.id)} />
        ))}
      </ChipRow>

      {showSubs && (
        <ChipRow>
          <Chip label="Alle" active={activeSub === "all"} onClick={() => onSubChange("all")} />
          {subs.map((c) => (
            <Chip key={c.id} label={c.name} active={activeSub === c.id} onClick={() => onSubChange(c.id)} />
          ))}
        </ChipRow>
      )}
    </div>
  )
}
