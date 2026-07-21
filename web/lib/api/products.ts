import type { ListResponse, ProductDTO, ProductSummaryDTO, ProductVariantDTO } from "@/types"
import { apiRequest } from "../api"
import { LOW_STOCK_THRESHOLD } from "../variants"

export interface ListProductsParams {
  categoryId?: string
  limit?: number
  offset?: number
  // Only products a customer can buy; admin/management views omit it.
  visible?: boolean
}

interface RawSlotOption {
  productId?: string
  id?: string
  name?: string
  priceCents?: number
  depositCents?: number
  image?: string | null
  description?: string | null
  jeton?: { id: string; name: string; color: string } | null
}

interface RawBundleSlot {
  id: string
  name: string
  sequence?: number
  options?: RawSlotOption[]
}

interface RawVariant {
  id: string
  value: string
  priceAdjustmentCents?: number
  isActive: boolean
  sequence: number
  stock?: number | null
}

interface RawProduct extends Omit<ProductDTO, "variants"> {
  stock?: number
  bundleSlots?: RawBundleSlot[]
  variants?: RawVariant[]
}

function normalizeProduct(raw: RawProduct): ProductDTO {
  const product = raw as ProductDTO

  const isBundle = raw.type === "bundle"
  const stock = raw.stock

  if (raw.variants && raw.variants.length > 0) {
    product.variants = raw.variants.map((v): ProductVariantDTO => {
      const variantStock = typeof v.stock === "number" ? v.stock : null
      return {
        id: v.id,
        value: v.value,
        priceAdjustmentCents: v.priceAdjustmentCents ?? 0,
        isActive: v.isActive,
        sequence: v.sequence,
        stock: variantStock,
        availableQuantity: variantStock,
        isAvailable: variantStock === null ? true : variantStock > 0,
        isLowStock: variantStock !== null && variantStock > 0 && variantStock <= LOW_STOCK_THRESHOLD,
      }
    })
    const active = product.variants.filter((v) => v.isActive)
    const anyActiveAvailable = active.some((v) => v.isAvailable !== false)
    product.isAvailable = anyActiveAvailable
    product.isLowStock = anyActiveAvailable && active.some((v) => v.isLowStock === true)
    product.availableQuantity = active.reduce((s, v) => s + (v.availableQuantity ?? 0), 0)
  } else if (!isBundle && typeof stock === "number") {
    product.availableQuantity = stock
    product.isAvailable = stock > 0
    product.isLowStock = stock > 0 && stock <= LOW_STOCK_THRESHOLD
  }

  const bundleSlots = raw.bundleSlots
  if (bundleSlots && bundleSlots.length > 0) {
    product.bundle = {
      slots: bundleSlots.map((slot) => ({
        id: slot.id,
        name: slot.name,
        sequence: slot.sequence ?? 0,
        options: Array.isArray(slot.options)
          ? slot.options.map((opt) => ({
              id: opt.productId ?? opt.id ?? "",
              name: opt.name ?? "",
              priceCents: opt.priceCents ?? 0,
              depositCents: opt.depositCents ?? 0,
              type: "simple" as const,
              image: opt.image ?? null,
              description: opt.description ?? null,
              isActive: true,
              isAvailable: true,
              category: null as unknown as ProductSummaryDTO["category"],
              jeton: opt.jeton ? { id: opt.jeton.id, name: opt.jeton.name, color: opt.jeton.color } : undefined,
            }))
          : null,
      })),
    }
  }

  return product
}

export async function listProducts(
  params: ListProductsParams = {},
  options: RequestInit = {}
): Promise<ListResponse<ProductDTO>> {
  const searchParams = new URLSearchParams()

  if (params.categoryId) {
    searchParams.append("category_id", params.categoryId)
  }

  if (params.limit !== undefined) {
    searchParams.append("limit", params.limit.toString())
  }

  if (params.offset !== undefined) {
    searchParams.append("offset", params.offset.toString())
  }

  if (params.visible) {
    searchParams.append("visible", "true")
  }

  const queryString = searchParams.toString()
  const endpoint = `/v1/products${queryString ? `?${queryString}` : ""}`

  const raw = await apiRequest<ListResponse<RawProduct>>(endpoint, options)
  const items = (raw.items || []).map(normalizeProduct)

  // Build stock map from simple products to apply to bundle slot options
  const stockMap = new Map<string, { stock: number; isAvailable: boolean; isLowStock: boolean }>()
  const simpleById = new Map<string, ProductDTO>()
  for (const p of items) {
    if (p.type !== "bundle") {
      simpleById.set(p.id, p)
      if (typeof p.availableQuantity === "number") {
        stockMap.set(p.id, {
          stock: p.availableQuantity,
          isAvailable: p.isAvailable !== false,
          isLowStock: p.isLowStock === true,
        })
      }
    }
  }

  // Apply stock + variants to bundle slot options: the option embed from the
  // backend is minimal, but the full products are in the same list, so the
  // bundle configurator can offer an option's variants without an extra fetch.
  for (const p of items) {
    if (p.type === "bundle" && p.bundle?.slots) {
      for (const slot of p.bundle.slots) {
        if (slot.options) {
          for (const opt of slot.options) {
            const stockInfo = stockMap.get(opt.id)
            if (stockInfo) {
              opt.availableQuantity = stockInfo.stock
              opt.isAvailable = stockInfo.isAvailable
              opt.isLowStock = stockInfo.isLowStock
            }
            const full = simpleById.get(opt.id)
            if (full?.variants?.length) {
              opt.variants = full.variants
              opt.variantLabel = full.variantLabel
            }
          }
        }
      }
    }
  }

  return { ...raw, items }
}

export async function setProductActive(token: string, productId: string, isActive: boolean): Promise<ProductDTO> {
  const res = await fetch(`/api/v1/pos/products/${encodeURIComponent(productId)}/active`, {
    method: "PATCH",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ isActive }),
  })
  if (!res.ok) throw new Error("active_update_failed")
  return (await res.json()) as ProductDTO
}
