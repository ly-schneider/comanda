"use client"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { type CategoryLike, childrenOf, topLevelCategories } from "@/lib/categories"

export function CategoryMultiSelect({
  categories,
  selectedIds,
  onChange,
  invalid,
}: {
  categories: CategoryLike[]
  selectedIds: string[]
  onChange: (ids: string[]) => void
  invalid?: boolean
}) {
  const tops = topLevelCategories(categories)

  function toggle(id: string, checked: boolean) {
    onChange(checked ? [...selectedIds, id] : selectedIds.filter((x) => x !== id))
  }

  function row(c: CategoryLike, child: boolean) {
    return (
      <label
        key={c.id}
        className={`hover:bg-muted flex cursor-pointer items-center gap-2 rounded-lg p-2 ${child ? "pl-7" : "font-medium"}`}
      >
        <Checkbox checked={selectedIds.includes(c.id)} onCheckedChange={(v) => toggle(c.id, v === true)} />
        <span>{c.name}</span>
      </label>
    )
  }

  return (
    <div className="space-y-2">
      <Label>Kategorien</Label>
      <div className="max-h-56 space-y-0.5 overflow-y-auto rounded-xl border p-2" aria-invalid={invalid || undefined}>
        {tops.length === 0 ? (
          <p className="text-muted-foreground p-2 text-sm">Keine Kategorien vorhanden</p>
        ) : (
          tops.map((top) => (
            <div key={top.id}>
              {row(top, false)}
              {childrenOf(categories, top.id).map((sub) => row(sub, true))}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
