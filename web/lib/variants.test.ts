import { describe, expect, it } from "vitest"
import type { ProductSummaryDTO, ProductVariantDTO } from "@/types"
import {
  activeVariants,
  findVariant,
  formatPriceAdjustment,
  hasVariants,
  variantPickerLabel,
  variantUnitPrice,
} from "./variants"

const makeVariant = (overrides: Partial<ProductVariantDTO> = {}): ProductVariantDTO => ({
  id: "v1",
  value: "M",
  priceAdjustmentCents: 0,
  isActive: true,
  sequence: 1,
  ...overrides,
})

const makeProduct = (overrides: Partial<ProductSummaryDTO> = {}): ProductSummaryDTO => ({
  id: "p1",
  category: { id: "c1", name: "Drinks" } as ProductSummaryDTO["category"],
  type: "simple",
  name: "T-Shirt",
  image: null,
  priceCents: 2000,
  depositCents: 0,
  isActive: true,
  ...overrides,
})

describe("hasVariants", () => {
  it("returns false when variants is absent", () => {
    expect(hasVariants(makeProduct())).toBe(false)
  })

  it("returns false for empty array", () => {
    expect(hasVariants(makeProduct({ variants: [] }))).toBe(false)
  })

  it("returns true when variants are present", () => {
    expect(hasVariants(makeProduct({ variants: [makeVariant()] }))).toBe(true)
  })
})

describe("activeVariants", () => {
  it("returns only active variants sorted by sequence", () => {
    const variants = [
      makeVariant({ id: "v1", isActive: true, sequence: 2 }),
      makeVariant({ id: "v2", isActive: false, sequence: 1 }),
      makeVariant({ id: "v3", isActive: true, sequence: 1 }),
    ]
    const result = activeVariants(makeProduct({ variants }))
    expect(result.map((v) => v.id)).toEqual(["v3", "v1"])
  })

  it("returns empty array when variants is absent", () => {
    expect(activeVariants(makeProduct())).toEqual([])
  })
})

describe("variantPickerLabel", () => {
  it("returns the variantLabel when set", () => {
    expect(variantPickerLabel(makeProduct({ variantLabel: "Grösse" }))).toBe("Grösse")
  })

  it("returns 'Variante' when variantLabel is null", () => {
    expect(variantPickerLabel(makeProduct({ variantLabel: null }))).toBe("Variante")
  })

  it("returns 'Variante' when variantLabel is absent", () => {
    expect(variantPickerLabel(makeProduct())).toBe("Variante")
  })
})

describe("findVariant", () => {
  const variants = [makeVariant({ id: "v1" }), makeVariant({ id: "v2" })]

  it("finds an existing variant by id", () => {
    expect(findVariant(makeProduct({ variants }), "v2")?.id).toBe("v2")
  })

  it("returns undefined for an unknown id", () => {
    expect(findVariant(makeProduct({ variants }), "v99")).toBeUndefined()
  })

  it("returns undefined when variantId is null", () => {
    expect(findVariant(makeProduct({ variants }), null)).toBeUndefined()
  })
})

describe("variantUnitPrice", () => {
  it("adds a positive adjustment", () => {
    expect(variantUnitPrice(makeProduct({ priceCents: 2000 }), makeVariant({ priceAdjustmentCents: 200 }))).toBe(2200)
  })

  it("subtracts a negative adjustment", () => {
    expect(variantUnitPrice(makeProduct({ priceCents: 2000 }), makeVariant({ priceAdjustmentCents: -200 }))).toBe(1800)
  })

  it("returns base price for zero adjustment", () => {
    expect(variantUnitPrice(makeProduct({ priceCents: 2000 }), makeVariant({ priceAdjustmentCents: 0 }))).toBe(2000)
  })
})

describe("formatPriceAdjustment", () => {
  it("returns empty string for zero", () => {
    expect(formatPriceAdjustment(0)).toBe("")
  })

  it("formats a positive adjustment", () => {
    expect(formatPriceAdjustment(200)).toBe("+ CHF 2.-")
  })

  it("formats a negative adjustment with minus sign", () => {
    expect(formatPriceAdjustment(-200)).toBe("− CHF 2.-")
  })

  it("formats fractional amounts", () => {
    expect(formatPriceAdjustment(150)).toBe("+ CHF 1.50")
  })
})
