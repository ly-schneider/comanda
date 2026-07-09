"use client"

import { Button } from "@/components/ui/button"

export type FilterCategory = { id: string; name: string }

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

// Horizontal category filter. Scrolls horizontally on mobile (no wrapping),
// wraps onto multiple lines from `sm` upwards.
export function CategoryFilter({
  categories,
  active,
  onChange,
}: {
  categories: FilterCategory[]
  active: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex [scrollbar-width:none] gap-2 overflow-x-auto pb-1 [-ms-overflow-style:none] sm:flex-wrap sm:overflow-x-visible sm:pb-0 [&::-webkit-scrollbar]:hidden">
      <Chip label="Alles" active={active === "all"} onClick={() => onChange("all")} />
      {categories.map((c) => (
        <Chip key={c.id} label={c.name} active={active === c.id} onClick={() => onChange(c.id)} />
      ))}
    </div>
  )
}
