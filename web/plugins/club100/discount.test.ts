import { describe, expect, it } from "vitest"
import type { CartItem } from "@/types/cart"
import { type Club100Member, computeClub100Discount } from "./discount"

const member = (remaining: number): Club100Member => ({
  id: "p1",
  firstName: "Alice",
  lastName: "A",
  remaining,
  max: 5,
})

const line = (id: string, productId: string, quantity: number, totalPriceCents: number): CartItem => ({
  id,
  product: { id: productId, name: productId } as CartItem["product"],
  quantity,
  totalPriceCents,
})

describe("computeClub100Discount", () => {
  it("returns null when no free products are configured", () => {
    expect(computeClub100Discount(member(3), [line("l1", "prd1", 1, 500)], [])).toBeNull()
  })

  it("returns null when the member has no redemptions left", () => {
    expect(computeClub100Discount(member(0), [line("l1", "prd1", 1, 500)], ["prd1"])).toBeNull()
  })

  it("returns null when the cart has no eligible products", () => {
    expect(computeClub100Discount(member(3), [line("l1", "prd9", 1, 500)], ["prd1"])).toBeNull()
  })

  it("discounts a single eligible line", () => {
    const d = computeClub100Discount(member(3), [line("l1", "prd1", 1, 500)], ["prd1"])
    expect(d?.totalDiscountCents).toBe(500)
    expect(d?.discountedItems).toHaveLength(1)
  })

  it("caps discounted quantity within a line at the remaining redemptions", () => {
    const d = computeClub100Discount(member(2), [line("l1", "prd1", 3, 400)], ["prd1"])
    expect(d?.discountedItems[0]?.discountedQuantity).toBe(2)
    expect(d?.totalDiscountCents).toBe(800)
  })

  it("stops once redemptions run out across multiple eligible lines", () => {
    const cart = [line("l1", "prd1", 1, 300), line("l2", "prd1", 1, 300), line("l3", "prd1", 1, 300)]
    const d = computeClub100Discount(member(2), cart, ["prd1"])
    expect(d?.discountedItems).toHaveLength(2)
    expect(d?.totalDiscountCents).toBe(600)
  })

  it("skips non-eligible lines while discounting eligible ones", () => {
    const cart = [line("l1", "prd9", 2, 999), line("l2", "prd1", 1, 250)]
    const d = computeClub100Discount(member(2), cart, ["prd1"])
    expect(d?.discountedItems).toHaveLength(1)
    expect(d?.discountedItems[0]?.productId).toBe("prd1")
    expect(d?.totalDiscountCents).toBe(250)
  })
})
