"use client"

import { useCallback, useState } from "react"
import { useInventoryStream } from "@/hooks/use-inventory-stream"
import { LOW_STOCK_THRESHOLD } from "@/lib/variants"
import type { CategoryDTO, ListResponse, ProductDTO } from "@/types"
import CatalogGrid from "./catalog-grid"

function applyStockToProduct(p: ProductDTO, stockMap: Map<string, number>): ProductDTO {
  let updated = p

  if (p.type === "bundle" && p.bundle?.slots) {
    updated = {
      ...p,
      bundle: {
        ...p.bundle,
        slots: p.bundle.slots.map((slot) => ({
          ...slot,
          options:
            slot.options?.map((opt) => {
              const optStock = stockMap.get(opt.id)
              if (optStock === undefined) return opt
              return {
                ...opt,
                availableQuantity: optStock,
                isAvailable: optStock > 0,
                isLowStock: optStock > 0 && optStock <= LOW_STOCK_THRESHOLD,
              }
            }) ?? null,
        })),
      },
    }
  } else {
    const newStock = stockMap.get(p.id)
    if (newStock !== undefined) {
      updated = {
        ...p,
        availableQuantity: newStock,
        isAvailable: newStock > 0,
        isLowStock: newStock > 0 && newStock <= LOW_STOCK_THRESHOLD,
      }
    }
  }

  return updated
}

export function CatalogGridLive({
  initialProducts,
  categories = [],
}: {
  initialProducts: ListResponse<ProductDTO>
  categories?: CategoryDTO[]
}) {
  const [products, setProducts] = useState(initialProducts)

  const handleStockUpdate = useCallback((stockMap: Map<string, number>) => {
    setProducts((prev) => ({
      ...prev,
      items: prev.items.map((p) => applyStockToProduct(p, stockMap)),
    }))
  }, [])

  useInventoryStream({ onUpdate: handleStockUpdate, enabled: true })

  return <CatalogGrid products={products} categories={categories} />
}
