import { describe, expect, it } from "vitest"
import { categoryScope, rangeBounds, scopeParams } from "@/lib/inventory-export"

describe("rangeBounds", () => {
  it("makes the picked end day inclusive by moving the bound to the next midnight", () => {
    const bounds = rangeBounds({ from: new Date(2026, 6, 1, 14, 30), to: new Date(2026, 6, 3, 9, 0) })
    expect(bounds?.from).toEqual(new Date(2026, 6, 1))
    expect(bounds?.to).toEqual(new Date(2026, 6, 4))
  })

  it("treats a single picked day as that whole day", () => {
    const bounds = rangeBounds({ from: new Date(2026, 6, 1), to: undefined })
    expect(bounds?.from).toEqual(new Date(2026, 6, 1))
    expect(bounds?.to).toEqual(new Date(2026, 6, 2))
  })

  it("spans a month boundary", () => {
    const bounds = rangeBounds({ from: new Date(2026, 6, 30), to: new Date(2026, 6, 31) })
    expect(bounds?.to).toEqual(new Date(2026, 7, 1))
  })

  it("returns null without a start", () => {
    expect(rangeBounds({ from: undefined, to: undefined })).toBeNull()
  })
})

describe("scopeParams", () => {
  it("serialises scope, id and both bounds", () => {
    const params = scopeParams("product", "prod00000001", {
      from: new Date(2026, 6, 1),
      to: new Date(2026, 6, 1),
    })
    expect(params?.get("scope")).toBe("product")
    expect(params?.get("scopeId")).toBe("prod00000001")
    expect(params?.get("from")).toBe(new Date(2026, 6, 1).toISOString())
    expect(params?.get("to")).toBe(new Date(2026, 6, 2).toISOString())
  })

  it("returns null without a target or a range", () => {
    expect(scopeParams("product", "", { from: new Date(2026, 6, 1), to: undefined })).toBeNull()
    expect(scopeParams("product", "prod00000001", { from: undefined, to: undefined })).toBeNull()
  })
})

describe("categoryScope", () => {
  const categories = [
    { id: "cat1", name: "Getränke", isActive: true, position: 0, parentId: null },
    { id: "cat2", name: "Bar Ost", isActive: true, position: 0, parentId: "cat1" },
  ]

  it("treats a top-level category as the whole subtree", () => {
    expect(categoryScope(categories, "cat1")).toBe("category")
  })

  it("treats a child as a subcategory", () => {
    expect(categoryScope(categories, "cat2")).toBe("subcategory")
  })

  it("falls back to the subtree scope for an unknown id", () => {
    expect(categoryScope(categories, "nope")).toBe("category")
  })
})
