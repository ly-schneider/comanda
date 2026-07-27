import type { DateRange } from "react-day-picker"
import type { CategoryLike } from "@/lib/categories"

export type InventoryScope = "product" | "subcategory" | "category"

function startOfDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

// The backend range is half-open, and a picked end date means "include that whole
// day", so the exclusive bound is the next midnight.
export function rangeBounds(range: DateRange): { from: Date; to: Date } | null {
  if (!range.from) return null
  const from = startOfDay(range.from)
  const end = startOfDay(range.to ?? range.from)
  const to = new Date(end.getFullYear(), end.getMonth(), end.getDate() + 1)
  return { from, to }
}

export function scopeParams(scope: InventoryScope, scopeId: string, range: DateRange): URLSearchParams | null {
  if (!scopeId) return null
  const bounds = rangeBounds(range)
  if (!bounds) return null

  return new URLSearchParams({
    scope,
    scopeId,
    from: bounds.from.toISOString(),
    to: bounds.to.toISOString(),
  })
}

// A top-level category covers its whole subtree; a child covers only itself.
export function categoryScope(categories: CategoryLike[], categoryId: string): InventoryScope {
  const category = categories.find((c) => c.id === categoryId)
  return category?.parentId ? "subcategory" : "category"
}
