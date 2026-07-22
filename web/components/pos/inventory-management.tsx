"use client"

import { Minus, Plus } from "lucide-react"
import Image from "next/image"
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { ProductFilterBar } from "@/components/catalog/product-filter-bar"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { useProductFilter } from "@/hooks/use-product-filter"
import { listCategories, setCategoryActive } from "@/lib/api/categories"
import { setProductActive } from "@/lib/api/products"
import { categoryOrderMap, childrenOf, compareByCategory, topLevelCategories } from "@/lib/categories"
import { cn } from "@/lib/utils"
import { activeVariants } from "@/lib/variants"
import type { CategoryDTO, ListResponse, ProductDTO } from "@/types"

type StockUpdatedHandler = (productId: string, newStock: number, variantId?: string) => void

function StockControl({
  product,
  variantId,
  currentStock,
  token,
  onStockUpdated,
}: {
  product: ProductDTO
  variantId?: string
  currentStock: number
  token: string
  onStockUpdated: StockUpdatedHandler
}) {
  const [localStock, setLocalStock] = useState(String(currentStock))
  const [saving, setSaving] = useState(false)
  const pendingRef = useRef<AbortController | null>(null)

  const adjustStock = useCallback(
    async (delta: number) => {
      pendingRef.current?.abort()

      const optimistic = Math.max(0, parseInt(localStock, 10) + delta)
      setLocalStock(String(optimistic))
      onStockUpdated(product.id, optimistic, variantId)

      const controller = new AbortController()
      pendingRef.current = controller
      setSaving(true)
      try {
        const res = await fetch(`/api/v1/pos/products/${product.id}/inventory`, {
          method: "PATCH",
          headers: {
            Authorization: `Bearer ${token}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ delta, reason: "manual_adjust", ...(variantId ? { variantId } : {}) }),
          signal: controller.signal,
        })
        if (res.ok) {
          const data = (await res.json()) as {
            quantity: number
            variants?: { variantId: string; quantity: number }[]
          }
          // For a variant product the top-level quantity is the product-level
          // aggregate; the variant's own stock lives in variants[].
          const next = variantId
            ? (data.variants?.find((v) => v.variantId === variantId)?.quantity ?? data.quantity)
            : data.quantity
          setLocalStock(String(next))
          onStockUpdated(product.id, next, variantId)
        } else {
          setLocalStock(String(currentStock))
          onStockUpdated(product.id, currentStock, variantId)
        }
      } catch (e) {
        if (e instanceof DOMException && e.name === "AbortError") return
        setLocalStock(String(currentStock))
        onStockUpdated(product.id, currentStock, variantId)
      } finally {
        setSaving(false)
      }
    },
    [product.id, variantId, token, onStockUpdated, localStock, currentStock]
  )

  const handleAbsoluteSet = useCallback(async () => {
    const target = parseInt(localStock, 10)
    if (isNaN(target) || target < 0) return
    const delta = target - currentStock
    if (delta === 0) return
    await adjustStock(delta)
  }, [localStock, currentStock, adjustStock])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") handleAbsoluteSet()
    },
    [handleAbsoluteSet]
  )

  return (
    <div className="flex items-center gap-1.5">
      <Button
        variant="outline"
        size="icon"
        className="size-9 shrink-0 rounded-[10px]"
        disabled={saving || parseInt(localStock, 10) <= 0}
        onClick={() => adjustStock(-1)}
      >
        <Minus className="size-4" />
      </Button>
      <Input
        type="number"
        min={0}
        value={localStock}
        onChange={(e) => setLocalStock(e.target.value)}
        onBlur={handleAbsoluteSet}
        onKeyDown={handleKeyDown}
        disabled={saving}
        className="bg-card h-9 w-16 [appearance:textfield] rounded-[10px] text-center [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
      />
      <Button
        variant="outline"
        size="icon"
        className="size-9 shrink-0 rounded-[10px]"
        disabled={saving}
        onClick={() => adjustStock(1)}
      >
        <Plus className="size-4" />
      </Button>
    </div>
  )
}

function ActiveToggle({
  product,
  token,
  onActiveUpdated,
}: {
  product: ProductDTO
  token: string
  onActiveUpdated: (productId: string, isActive: boolean) => void
}) {
  const isActive = product.isActive !== false
  const [saving, setSaving] = useState(false)

  const handleChange = useCallback(
    async (checked: boolean) => {
      setSaving(true)
      onActiveUpdated(product.id, checked)
      try {
        await setProductActive(token, product.id, checked)
      } catch {
        onActiveUpdated(product.id, !checked)
      } finally {
        setSaving(false)
      }
    },
    [product.id, token, onActiveUpdated]
  )

  return (
    <div className="flex items-center gap-2">
      <span className="text-muted-foreground text-xs font-medium">Aktiv</span>
      <Switch checked={isActive} disabled={saving} onCheckedChange={handleChange} />
    </div>
  )
}

function CategoryRow({
  category,
  token,
  parentInactive,
  onActiveUpdated,
}: {
  category: CategoryDTO
  token: string
  parentInactive?: boolean
  onActiveUpdated: (categoryId: string, isActive: boolean) => void
}) {
  const [saving, setSaving] = useState(false)

  const handleChange = useCallback(
    async (checked: boolean) => {
      setSaving(true)
      onActiveUpdated(category.id, checked)
      try {
        await setCategoryActive(token, category.id, checked)
      } catch {
        onActiveUpdated(category.id, !checked)
      } finally {
        setSaving(false)
      }
    },
    [category.id, token, onActiveUpdated]
  )

  return (
    <div
      className={cn(
        "flex items-center justify-between gap-2 rounded-xl border p-3",
        (!category.isActive || parentInactive) && "opacity-60"
      )}
    >
      <div className="flex min-w-0 items-center gap-2">
        <p className="truncate text-sm font-medium">{category.name}</p>
        {parentInactive && category.isActive && (
          <span className="text-muted-foreground shrink-0 text-xs">Oberkategorie inaktiv</span>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <span className="text-muted-foreground text-xs font-medium">Aktiv</span>
        <Switch checked={category.isActive} disabled={saving} onCheckedChange={handleChange} />
      </div>
    </div>
  )
}

// Deactivating a category makes every product under it (and its subcategories)
// unbuyable — same cascade the admin toggle triggers.
function CategoriesSection({
  categories,
  token,
  onActiveUpdated,
}: {
  categories: CategoryDTO[]
  token: string
  onActiveUpdated: (categoryId: string, isActive: boolean) => void
}) {
  const topLevel = topLevelCategories(categories)

  if (topLevel.length === 0) {
    return <p className="text-muted-foreground py-4 text-center text-sm">Keine Kategorien gefunden.</p>
  }

  return (
    <div className="flex flex-col gap-2 md:grid md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {topLevel.map((cat) => {
        const children = childrenOf(categories, cat.id)
        return (
          <div key={cat.id} className="flex flex-col gap-2">
            <CategoryRow category={cat} token={token} onActiveUpdated={onActiveUpdated} />
            {children.length > 0 && (
              <div className="flex flex-col gap-2 pl-4">
                {children.map((child) => (
                  <CategoryRow
                    key={child.id}
                    category={child}
                    token={token}
                    parentInactive={!cat.isActive}
                    onActiveUpdated={onActiveUpdated}
                  />
                ))}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

const ItemCard = memo(function ItemCard({
  product,
  token,
  onStockUpdated,
  onActiveUpdated,
}: {
  product: ProductDTO
  token: string
  onStockUpdated: StockUpdatedHandler
  onActiveUpdated: (productId: string, isActive: boolean) => void
}) {
  const isActive = product.isActive !== false
  const isBundle = product.type === "bundle"
  const variants = isBundle ? [] : activeVariants(product)
  const stock = product.availableQuantity ?? 0

  return (
    <div className={cn("flex items-start gap-4 rounded-xl border p-3", !isActive && "opacity-60")}>
      <div className="relative size-20 shrink-0 overflow-hidden rounded-xl bg-[#cec9c6]">
        {product.image && (
          <Image
            src={product.image}
            alt={product.name}
            fill
            sizes="80px"
            quality={90}
            className="object-cover"
            unoptimized
          />
        )}
        {!isActive && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/30">
            <span className="rounded-full bg-black/70 px-2 py-0.5 text-[10px] font-semibold text-white">Inaktiv</span>
          </div>
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex items-start justify-between gap-2">
          <p className="truncate text-sm font-medium">{product.name}</p>
          <ActiveToggle product={product} token={token} onActiveUpdated={onActiveUpdated} />
        </div>
        {isBundle ? (
          <span className="text-muted-foreground text-xs">Bundle</span>
        ) : variants.length > 0 ? (
          <div className="flex flex-col gap-1.5">
            {variants.map((v) => (
              <div key={v.id} className="flex items-center justify-between gap-2">
                <span className="text-muted-foreground min-w-0 truncate text-xs">{v.value}</span>
                <StockControl
                  product={product}
                  variantId={v.id}
                  currentStock={v.availableQuantity ?? v.stock ?? 0}
                  token={token}
                  onStockUpdated={onStockUpdated}
                />
              </div>
            ))}
          </div>
        ) : (
          <StockControl product={product} currentStock={stock} token={token} onStockUpdated={onStockUpdated} />
        )}
      </div>
    </div>
  )
})

export function InventoryManagement({
  products,
  token,
  onStockUpdated,
  onActiveUpdated,
}: {
  products: ListResponse<ProductDTO>
  token: string
  onStockUpdated: StockUpdatedHandler
  onActiveUpdated: (productId: string, isActive: boolean) => void
}) {
  const [categories, setCategories] = useState<CategoryDTO[]>([])
  const { query, setQuery, activeCat, activeSub, selectCat, setActiveSub, matches } = useProductFilter()

  useEffect(() => {
    listCategories()
      .then(setCategories)
      .catch(() => {})
  }, [])

  const handleCategoryActiveUpdated = useCallback((categoryId: string, isActive: boolean) => {
    setCategories((prev) => prev.map((c) => (c.id === categoryId ? { ...c, isActive } : c)))
  }, [])

  const order = useMemo(() => categoryOrderMap(categories), [categories])
  const activeCategories = useMemo(() => categories.filter((c) => c.isActive), [categories])

  const byCategory = useCallback(
    (a: ProductDTO, b: ProductDTO) => {
      const byCat = compareByCategory(order, a.category?.id, b.category?.id)
      return byCat !== 0 ? byCat : a.name.localeCompare(b.name, "de")
    },
    [order]
  )

  const simpleProducts = products.items.filter((p) => p.type !== "bundle" && matches(p)).sort(byCategory)
  const bundles = products.items.filter((p) => p.type === "bundle" && matches(p)).sort(byCategory)

  const renderGrid = (items: ProductDTO[]) => (
    <div className="flex flex-col gap-2 md:grid md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {items.map((product) => (
        <ItemCard
          key={product.id}
          product={product}
          token={token}
          onStockUpdated={onStockUpdated}
          onActiveUpdated={onActiveUpdated}
        />
      ))}
    </div>
  )

  return (
    <div className="h-full space-y-6 overflow-y-auto px-3 pb-4 md:px-4">
      <ProductFilterBar
        query={query}
        onQueryChange={setQuery}
        categories={activeCategories}
        activeCat={activeCat}
        activeSub={activeSub}
        onSelectCat={selectCat}
        onSubChange={setActiveSub}
      />

      <div className="space-y-3">
        <h1 className="text-xl font-semibold">Produkte verwalten</h1>
        {simpleProducts.length > 0 ? (
          renderGrid(simpleProducts)
        ) : (
          <p className="text-muted-foreground py-4 text-center text-sm">Keine Produkte gefunden.</p>
        )}
      </div>

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">Bundles verwalten</h2>
        {bundles.length > 0 ? (
          renderGrid(bundles)
        ) : (
          <p className="text-muted-foreground py-4 text-center text-sm">Keine Bundles gefunden.</p>
        )}
      </div>

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">Kategorien verwalten</h2>
        <CategoriesSection categories={categories} token={token} onActiveUpdated={handleCategoryActiveUpdated} />
      </div>
    </div>
  )
}
