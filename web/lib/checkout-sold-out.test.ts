import { beforeEach, describe, expect, it } from "vitest"
import {
  buildSoldOutMap,
  extractSoldOutItems,
  extractUnavailableItems,
  soldOutErrorForItem,
  soldOutKey,
  soldOutMessage,
  stashSoldOut,
  takeSoldOut,
} from "./checkout-sold-out"

describe("checkout-sold-out", () => {
  beforeEach(() => sessionStorage.clear())

  it("keys by product and variant, treating no variant as empty", () => {
    expect(soldOutKey("p1", "v1")).toBe("p1::v1")
    expect(soldOutKey("p1")).toBe("p1::")
    expect(soldOutKey("p1", null)).toBe("p1::")
  })

  it("messages from remaining stock", () => {
    expect(soldOutMessage(0)).toBe("Ausverkauft")
    expect(soldOutMessage(undefined)).toBe("Ausverkauft")
    expect(soldOutMessage(3)).toBe("Nur noch 3 verfügbar")
  })

  it("messages deactivated items independent of stock", () => {
    expect(soldOutMessage(0, true)).toBe("Nicht mehr verfügbar")
    expect(soldOutMessage(5, true)).toBe("Nicht mehr verfügbar")
  })

  it("extracts unavailable items tagged unavailable", () => {
    expect(extractUnavailableItems({ items: [{ productId: "p1", name: "Cola" }] })).toEqual([
      { productId: "p1", name: "Cola", unavailable: true },
    ])
    expect(extractUnavailableItems(undefined)).toEqual([])
  })

  it("extracts only well-formed items from an error's details", () => {
    expect(extractSoldOutItems(undefined)).toEqual([])
    expect(extractSoldOutItems({ items: "nope" })).toEqual([])
    expect(
      extractSoldOutItems({ items: [{ productId: "p1", variantId: "v1", available: 2 }, { bogus: true }] })
    ).toEqual([{ productId: "p1", variantId: "v1", available: 2 }])
  })

  it("stashes then takes once (clearing the stash)", () => {
    stashSoldOut([{ productId: "p1", available: 0 }])
    expect(takeSoldOut()).toEqual([{ productId: "p1", available: 0 }])
    expect(takeSoldOut()).toEqual([])
  })

  it("does not stash an empty list", () => {
    stashSoldOut([])
    expect(takeSoldOut()).toEqual([])
  })

  describe("soldOutErrorForItem", () => {
    const bundleLine = {
      product: { id: "bundle1" },
      configuration: { slotA: "crepe1", slotB: "cola1" },
      configurationVariants: { slotA: { id: "vGross" } },
    }

    it("matches a simple line on its own product+variant", () => {
      const map = buildSoldOutMap([{ productId: "shirt1", variantId: "vM", available: 0 }])
      expect(soldOutErrorForItem(map, { product: { id: "shirt1" }, variantId: "vM" })).toBe("Ausverkauft")
      expect(soldOutErrorForItem(map, { product: { id: "shirt1" }, variantId: "vL" })).toBeUndefined()
    })

    it("matches a bundle line via a sold-out component, prefixed with its name", () => {
      const map = buildSoldOutMap([{ productId: "cola1", name: "Cola", available: 2 }])
      expect(soldOutErrorForItem(map, bundleLine)).toBe("Cola: Nur noch 2 verfügbar")
    })

    it("matches a bundle component only on the configured variant", () => {
      const gross = buildSoldOutMap([{ productId: "crepe1", variantId: "vGross", name: "Crêpe (Gross)", available: 0 }])
      expect(soldOutErrorForItem(gross, bundleLine)).toBe("Crêpe (Gross): Ausverkauft")

      const klein = buildSoldOutMap([{ productId: "crepe1", variantId: "vKlein", available: 0 }])
      expect(soldOutErrorForItem(klein, bundleLine)).toBeUndefined()
    })

    it("marks deactivated components", () => {
      const map = buildSoldOutMap([{ productId: "cola1", name: "Cola", unavailable: true }])
      expect(soldOutErrorForItem(map, bundleLine)).toBe("Cola: Nicht mehr verfügbar")
    })

    it("returns undefined when nothing in the line is affected", () => {
      const map = buildSoldOutMap([{ productId: "other1", available: 0 }])
      expect(soldOutErrorForItem(map, bundleLine)).toBeUndefined()
    })
  })
})
