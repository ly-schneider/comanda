"use client"

import { Search } from "lucide-react"
import { CategoryFilter, type FilterCategory } from "@/components/catalog/category-filter"
import { Input } from "@/components/ui/input"

export function ProductFilterBar({
  query,
  onQueryChange,
  categories,
  activeCat,
  activeSub,
  onSelectCat,
  onSubChange,
}: {
  query: string
  onQueryChange: (value: string) => void
  categories: FilterCategory[]
  activeCat: string
  activeSub: string
  onSelectCat: (value: string) => void
  onSubChange: (value: string) => void
}) {
  return (
    <div className="space-y-3">
      <div className="relative">
        <Search className="text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2" />
        <Input
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          placeholder="Produkt suchen"
          className="pl-9"
          autoComplete="off"
        />
      </div>
      {categories.length > 0 && (
        <CategoryFilter
          categories={categories}
          active={activeCat}
          onChange={onSelectCat}
          activeSub={activeSub}
          onSubChange={onSubChange}
        />
      )}
    </div>
  )
}
