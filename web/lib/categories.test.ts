import { describe, expect, it } from "vitest"
import {
  type CategoryLike,
  categoryOrderMap,
  childrenOf,
  compareByCategory,
  hasChildren,
  topLevelCategories,
} from "./categories"

const cat = (id: string, position: number, parentId: string | null = null, name = id): CategoryLike => ({
  id,
  name,
  position,
  parentId,
})

describe("topLevelCategories", () => {
  it("keeps only top-level categories, sorted by position", () => {
    const cats = [cat("b", 1), cat("a", 0), cat("a1", 0, "a")]
    expect(topLevelCategories(cats).map((c) => c.id)).toEqual(["a", "b"])
  })

  it("breaks position ties by name", () => {
    const cats = [cat("z", 0, null, "Zucker"), cat("a", 0, null, "Apfel")]
    expect(topLevelCategories(cats).map((c) => c.id)).toEqual(["a", "z"])
  })
})

describe("childrenOf", () => {
  it("returns only the given parent's children, sorted by position", () => {
    const cats = [cat("a", 0), cat("a2", 1, "a"), cat("a1", 0, "a"), cat("b1", 0, "b")]
    expect(childrenOf(cats, "a").map((c) => c.id)).toEqual(["a1", "a2"])
  })

  it("is empty for a leaf category", () => {
    expect(childrenOf([cat("a", 0)], "a")).toEqual([])
  })
})

describe("hasChildren", () => {
  it("reflects whether a category has subcategories", () => {
    const cats = [cat("a", 0), cat("a1", 0, "a")]
    expect(hasChildren(cats, "a")).toBe(true)
    expect(hasChildren(cats, "a1")).toBe(false)
  })
})

describe("categoryOrderMap", () => {
  it("puts a top-level category's own products ahead of its children", () => {
    const order = categoryOrderMap([cat("a", 0), cat("a1", 0, "a")])
    expect(order.get("a")).toEqual([0, -1])
    expect(order.get("a1")).toEqual([0, 0])
  })

  it("ranks an orphan child (missing parent) last within the parent axis", () => {
    const order = categoryOrderMap([cat("x", 3, "gone")])
    expect(order.get("x")).toEqual([1_000_000, 3])
  })
})

describe("compareByCategory", () => {
  // A(0) { A1(0), A2(1) }   B(1) { B1(0) }
  const order = categoryOrderMap([cat("A", 0), cat("A1", 0, "A"), cat("A2", 1, "A"), cat("B", 1), cat("B1", 0, "B")])

  it("groups products under their parent, parent's own products first", () => {
    const sorted = ["B1", "A2", "A", "B", "A1"].sort((x, y) => compareByCategory(order, x, y))
    expect(sorted).toEqual(["A", "A1", "A2", "B", "B1"])
  })

  it("sorts known categories before unknown or missing ones, which tie", () => {
    expect(compareByCategory(order, "A", "nope")).toBeLessThan(0)
    expect(compareByCategory(order, undefined, "A")).toBeGreaterThan(0)
    expect(compareByCategory(order, "nope", undefined)).toBe(0)
  })
})
