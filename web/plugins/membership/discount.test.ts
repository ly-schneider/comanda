import { describe, expect, it } from "vitest"
import type { CartItem } from "@/types/cart"
import { computeMembershipDiscount } from "./discount"

const line = (id: string, productId: string, quantity: number, unitPriceCents: number): CartItem => ({
  id,
  product: { id: productId, name: productId } as CartItem["product"],
  quantity,
  totalPriceCents: unitPriceCents,
})

describe("computeMembershipDiscount", () => {
  it("gives nothing away when the tenant configured no free products", () => {
    expect(computeMembershipDiscount([line("l1", "burger", 1, 900)], [], 1)).toBeNull()
  })

  it("gives nothing away when the budget is used up", () => {
    expect(computeMembershipDiscount([line("l1", "burger", 1, 900)], ["burger"], 0)).toBeNull()
  })

  it("gives nothing away when the basket holds nothing eligible", () => {
    expect(computeMembershipDiscount([line("l1", "beer", 1, 700)], ["burger"], 1)).toBeNull()
  })

  it("takes the dearest eligible unit first", () => {
    const discount = computeMembershipDiscount(
      [line("l1", "fries", 1, 500), line("l2", "burger", 1, 900)],
      ["fries", "burger"],
      1
    )

    expect(discount?.freeQuantity).toBe(1)
    expect(discount?.totalDiscountCents).toBe(900)
    expect(discount?.items).toEqual([
      { cartItemId: "l2", productId: "burger", productName: "burger", freeQuantity: 1, unitPriceCents: 900 },
    ])
  })

  it("spreads a larger budget over several lines", () => {
    const discount = computeMembershipDiscount(
      [line("l1", "burger", 2, 900), line("l2", "fries", 2, 500)],
      ["burger", "fries"],
      3
    )

    expect(discount?.freeQuantity).toBe(3)
    expect(discount?.totalDiscountCents).toBe(900 * 2 + 500)
    expect(discount?.items.map((i) => [i.productId, i.freeQuantity])).toEqual([
      ["burger", 2],
      ["fries", 1],
    ])
  })

  it("never gives away more units than the basket holds", () => {
    const discount = computeMembershipDiscount([line("l1", "burger", 1, 900)], ["burger"], 5)

    expect(discount?.freeQuantity).toBe(1)
    expect(discount?.totalDiscountCents).toBe(900)
  })
})
