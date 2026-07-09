"use client"

import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { type CategoryLike, childrenOf, topLevelCategories } from "@/lib/categories"

const NO_SUB = "__none__"

export function CategorySelect({
  categories,
  categoryId,
  onChange,
  idPrefix,
  invalid,
}: {
  categories: CategoryLike[]
  categoryId: string
  onChange: (categoryId: string) => void
  idPrefix: string
  invalid?: boolean
}) {
  const byId = new Map(categories.map((c) => [c.id, c]))
  const node = categoryId ? byId.get(categoryId) : undefined
  const topId = node ? (node.parentId ?? node.id) : ""
  const subId = node?.parentId ? node.id : ""

  const tops = topLevelCategories(categories)
  const subs = topId ? childrenOf(categories, topId) : []

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor={`${idPrefix}-category`}>Kategorie</Label>
        <Select value={topId || undefined} onValueChange={onChange}>
          <SelectTrigger id={`${idPrefix}-category`} className="h-10" aria-invalid={invalid}>
            <SelectValue placeholder="Kategorie wählen" />
          </SelectTrigger>
          <SelectContent>
            {tops.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {subs.length > 0 && (
        <div className="space-y-1">
          <Label htmlFor={`${idPrefix}-subcategory`}>Unterkategorie</Label>
          <Select value={subId || NO_SUB} onValueChange={(v) => onChange(v === NO_SUB ? topId : v)}>
            <SelectTrigger id={`${idPrefix}-subcategory`} className="h-10">
              <SelectValue placeholder="Unterkategorie wählen" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_SUB}>Alle (Hauptkategorie)</SelectItem>
              {subs.map((c) => (
                <SelectItem key={c.id} value={c.id}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  )
}
