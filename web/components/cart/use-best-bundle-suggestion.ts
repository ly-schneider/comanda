"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useCart } from "@/contexts/cart-context"
import { listProducts } from "@/lib/api/products"
import { BundleSuggestion, findBestBundleSuggestion } from "@/lib/bundle-suggestions"
import type { ProductDTO } from "@/types"

export interface BestBundleSuggestionState {
  suggestion: BundleSuggestion | null
  allProductsReady: boolean
  contiguous: boolean
  startIndex: number
  endIndex: number
  dismissSuggestion: () => void
}

export function useBestBundleSuggestion(products?: ProductDTO[]): BestBundleSuggestionState {
  const { cart } = useCart()
  const [allProducts, setAllProducts] = useState<ProductDTO[] | null>(products ?? null)
  const [rawSuggestion, setRawSuggestion] = useState<BundleSuggestion | null>(null)
  const [dismissedBundleId, setDismissedBundleId] = useState<string | null>(null)
  const [dismissedCartKey, setDismissedCartKey] = useState<string | null>(null)

  const cartKey = useMemo(() => cart.items.map((i) => `${i.product.id}:${i.quantity}`).join(","), [cart.items])

  useEffect(() => {
    if (dismissedBundleId && cartKey !== dismissedCartKey) {
      setDismissedBundleId(null)
      setDismissedCartKey(null)
    }
  }, [cartKey, dismissedBundleId, dismissedCartKey])

  useEffect(() => {
    let cancelled = false
    async function ensureProducts() {
      if (allProducts) return
      try {
        const res = await listProducts()
        if (!cancelled) setAllProducts(res.items)
      } catch {
        if (!cancelled) setAllProducts([])
      }
    }
    ensureProducts()
    return () => {
      cancelled = true
    }
  }, [allProducts])

  useEffect(() => {
    if (!allProducts) return
    const best = findBestBundleSuggestion(cart, allProducts)
    setRawSuggestion(best)
  }, [cart, allProducts])

  const suggestion = useMemo(() => {
    if (!rawSuggestion) return null
    if (dismissedBundleId && rawSuggestion.bundleProduct.id === dismissedBundleId) return null
    return rawSuggestion
  }, [rawSuggestion, dismissedBundleId])

  const dismissSuggestion = useCallback(() => {
    if (rawSuggestion) {
      setDismissedBundleId(rawSuggestion.bundleProduct.id)
      setDismissedCartKey(cartKey)
    }
  }, [rawSuggestion, cartKey])

  const { contiguous, startIndex, endIndex } = useMemo(() => {
    if (!suggestion) return { contiguous: false, startIndex: -1, endIndex: -1 }
    const involvedIds = new Set(suggestion.sourceItems.map((s) => s.cartItem.id))
    const indices: number[] = []
    cart.items.forEach((it, idx) => {
      if (involvedIds.has(it.id)) indices.push(idx)
    })
    if (indices.length !== suggestion.sourceItems.length) return { contiguous: false, startIndex: -1, endIndex: -1 }
    const min = Math.min(...indices)
    const max = Math.max(...indices)
    for (let i = min; i <= max; i++) {
      const id = cart.items[i]?.id
      if (!id || !involvedIds.has(id)) return { contiguous: false, startIndex: -1, endIndex: -1 }
    }
    return { contiguous: true, startIndex: min, endIndex: max }
  }, [cart.items, suggestion])

  return { suggestion, allProductsReady: !!allProducts, contiguous, startIndex, endIndex, dismissSuggestion }
}
