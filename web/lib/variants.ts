import type { ProductSummaryDTO, ProductVariantDTO } from "@/types"
import { formatChf } from "./utils"

export function hasVariants(product: Pick<ProductSummaryDTO, "variants">): boolean {
  return Array.isArray(product.variants) && product.variants.length > 0
}

export function activeVariants(product: Pick<ProductSummaryDTO, "variants">): ProductVariantDTO[] {
  if (!product.variants) return []
  return [...product.variants].filter((v) => v.isActive).sort((a, b) => a.sequence - b.sequence)
}

export function variantPickerLabel(product: Pick<ProductSummaryDTO, "variantLabel">): string {
  return product.variantLabel || "Variante"
}

export function findVariant(
  product: Pick<ProductSummaryDTO, "variants">,
  variantId: string | undefined | null
): ProductVariantDTO | undefined {
  if (!variantId || !product.variants) return undefined
  return product.variants.find((v) => v.id === variantId)
}

export function variantUnitPrice(
  product: Pick<ProductSummaryDTO, "priceCents">,
  variant: Pick<ProductVariantDTO, "priceAdjustmentCents">
): number {
  return product.priceCents + variant.priceAdjustmentCents
}

export function formatPriceAdjustment(cents: number): string {
  if (cents === 0) return ""
  const abs = Math.abs(cents)
  const sign = cents > 0 ? "+" : "−"
  return `${sign} ${formatChf(abs)}`
}
