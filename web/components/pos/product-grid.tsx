"use client"

import { useEffect, useState } from "react"
import { CategoryFilter } from "@/components/catalog/category-filter"
import { ProductCardPOS } from "@/components/pos/product-card-pos"
import { listCategories } from "@/lib/api/categories"
import type { CategoryDTO, ListResponse, ProductDTO } from "@/types"

export function ProductGrid({
  products,
  onConfigure,
}: {
  products: ListResponse<ProductDTO>
  onConfigure: (p: ProductDTO) => void
}) {
  const [activeCat, setActiveCat] = useState<string>("all")
  const [categories, setCategories] = useState<CategoryDTO[]>([])

  useEffect(() => {
    listCategories()
      .then((cats) => setCategories(cats.filter((c) => c.isActive)))
      .catch(() => {})
  }, [])

  const getCatPos = (c?: { position?: number | null } | null) => {
    const v = c?.position
    return typeof v === "number" && isFinite(v) ? v : 1_000_000
  }

  const activeItems = products.items.filter((it) => it.isActive !== false)

  const sortedProducts = [...activeItems].sort((a, b) => {
    const pa = getCatPos(a.category)
    const pb = getCatPos(b.category)
    if (pa !== pb) return pa - pb
    return a.name.localeCompare(b.name)
  })

  const filtered = activeCat === "all" ? sortedProducts : sortedProducts.filter((it) => it.category?.id === activeCat)

  return (
    <main className="container mx-auto h-full space-y-3 overflow-y-auto px-3 pb-4 md:px-4">
      <h1 className="mb-1 text-xl font-semibold">Sortiment</h1>
      <CategoryFilter categories={categories} active={activeCat} onChange={setActiveCat} />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-4">
        {filtered.map((product) => (
          <ProductCardPOS key={product.id} product={product} onConfigure={() => onConfigure(product)} />
        ))}
      </div>
    </main>
  )
}
