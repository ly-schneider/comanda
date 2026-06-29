import { describe, expect, it } from "vitest"
import type { CartItem } from "@/types/cart"
import { cartDepositCents, cn, formatChf } from "./utils"

function item(depositCents: number, quantity: number): CartItem {
  return {
    id: `${depositCents}-${quantity}`,
    product: { id: "p", priceCents: 200, depositCents } as CartItem["product"],
    quantity,
    totalPriceCents: 200,
  }
}

describe("utils", () => {
  it("cn merges class names", () => {
    expect(cn("a", false && "b", "c")).toBe("a c")
  })
  it("formatChf formats cents to CHF", () => {
    expect(formatChf(0)).toContain("CHF")
    expect(formatChf(1990)).toMatch(/19/) // 19.90
  })

  describe("cartDepositCents", () => {
    it("returns 0 for an empty cart", () => {
      expect(cartDepositCents([])).toBe(0)
    })
    it("ignores products without a deposit", () => {
      expect(cartDepositCents([item(0, 3)])).toBe(0)
    })
    it("scales the deposit per unit", () => {
      expect(cartDepositCents([item(200, 3)])).toBe(600)
    })
    it("sums deposits across items", () => {
      expect(cartDepositCents([item(200, 1), item(0, 2), item(50, 4)])).toBe(400)
    })
  })
})
