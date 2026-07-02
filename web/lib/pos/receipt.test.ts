import { describe, expect, it } from "vitest"
import { buildCardCheckoutSummary, type Receipt } from "./receipt"

type Items = NonNullable<Receipt["items"]>

describe("buildCardCheckoutSummary", () => {
  it("joins quantity and title into a compact title", () => {
    const items: Items = [
      { title: "Bratwurst", quantity: 2, unitPriceCents: 800 },
      { title: "Cola", quantity: 1, unitPriceCents: 350 },
    ]
    const { title, details } = buildCardCheckoutSummary(items)
    expect(title).toBe("2× Bratwurst, 1× Cola")
    expect(details).toBe("2× Bratwurst\n1× Cola")
  })

  it("appends menu selections to the detail lines only", () => {
    const items: Items = [
      {
        title: "Menü",
        quantity: 1,
        unitPriceCents: 1500,
        configuration: [
          { slot: "Hauptgang", choice: "Bratwurst" },
          { slot: "Getränk", choice: "Cola" },
        ],
      },
    ]
    const { title, details } = buildCardCheckoutSummary(items)
    expect(title).toBe("1× Menü")
    expect(details).toBe("1× Menü (Hauptgang: Bratwurst, Getränk: Cola)")
  })

  it("returns empty strings for an empty cart", () => {
    expect(buildCardCheckoutSummary([])).toEqual({ title: "", details: "" })
  })
})
