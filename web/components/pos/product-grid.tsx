"use client"

import { useEffect, useMemo, useState } from "react"
import { CategoryFilter } from "@/components/catalog/category-filter"
import { ProductCardPOS } from "@/components/pos/product-card-pos"
import { listCategories } from "@/lib/api/categories"
import { categoryOrderMap, compareByCategory, effectivelyActiveCategories } from "@/lib/categories"
import type { CategoryDTO, ListResponse, ProductDTO } from "@/types"

export function ProductGrid({
  products,
  onConfigure,
}: {
  products: ListResponse<ProductDTO>
  onConfigure: (p: ProductDTO) => void
}) {
  const [activeCat, setActiveCat] = useState<string>("all")
  const [activeSub, setActiveSub] = useState<string>("all")
  const [categories, setCategories] = useState<CategoryDTO[]>([])
  const [categoriesLoaded, setCategoriesLoaded] = useState(false)

  useEffect(() => {
    listCategories()
      .then((cats) => {
        setCategories(effectivelyActiveCategories(cats))
        setCategoriesLoaded(true)
      })
      .catch(() => {})
  }, [])

  const order = useMemo(() => categoryOrderMap(categories), [categories])
  const activeCategoryIds = useMemo(() => new Set(categories.map((c) => c.id)), [categories])

  // Until categories load (or if the fetch fails) show all active products; the
  // backend still blocks deactivated items at checkout.
  const activeItems = products.items.filter((it) => {
    if (it.isActive === false) return false
    if (!categoriesLoaded) return true
    return it.category?.id != null && activeCategoryIds.has(it.category.id)
  })

  const sortedProducts = [...activeItems].sort((a, b) => {
    const byCat = compareByCategory(order, a.category?.id, b.category?.id)
    return byCat !== 0 ? byCat : a.name.localeCompare(b.name)
  })

  const filtered = sortedProducts.filter((it) => {
    if (activeCat === "all") return true
    if (activeSub !== "all") return it.category?.id === activeSub
    return it.category?.id === activeCat || it.category?.parentId === activeCat
  })

  const selectCat = (value: string) => {
    setActiveCat(value)
    setActiveSub("all")
  }

  return (
    <main className="container mx-auto h-full space-y-3 overflow-y-auto px-3 pb-4 md:px-4">
      <h1 className="mb-1 text-xl font-semibold">Sortiment</h1>
      <CategoryFilter
        categories={categories}
        active={activeCat}
        onChange={selectCat}
        activeSub={activeSub}
        onSubChange={setActiveSub}
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-4">
        {filtered.map((product) => (
          <ProductCardPOS key={product.id} product={product} onConfigure={() => onConfigure(product)} />
        ))}
      </div>
    </main>
  )
}
