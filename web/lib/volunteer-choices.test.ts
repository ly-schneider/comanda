import { describe, expect, it } from "vitest"
import { campaignPositions, positionOptions } from "@/lib/volunteer-choices"

const p = (id: string, choiceGroup: number, optionGroup = 1) => ({ id, choiceGroup, optionGroup })

type TestProduct = { id: string; choiceGroup: number; optionGroup: number }

const shape = (products: TestProduct[]) =>
  campaignPositions(products).map((pos) => pos.branches.map((b) => b.options.map((o) => o.id)))

describe("campaignPositions", () => {
  it("gives every ungrouped product its own position", () => {
    expect(shape([p("pommes", 0), p("getraenk", 0)])).toEqual([[["pommes"]], [["getraenk"]]])
  })

  it("collapses one branch into a single choice", () => {
    expect(shape([p("wurst", 0), p("pommes", 1), p("getraenk", 1)])).toEqual([[["wurst"]], [["pommes", "getraenk"]]])
  })

  it("keeps branches of one choice apart", () => {
    // Pommes ODER a drink: one position, two branches, drinks stay together.
    expect(shape([p("pommes", 1, 1), p("cola", 1, 2), p("wasser", 1, 2)])).toEqual([[["pommes"], ["cola", "wasser"]]])
  })

  it("keeps separate choices apart", () => {
    expect(shape([p("pommes", 1), p("bier", 2), p("salat", 1), p("wasser", 2)])).toEqual([
      [["pommes", "salat"]],
      [["bier", "wasser"]],
    ])
  })

  it("flattens a position to its products for the station", () => {
    const [position] = campaignPositions([p("pommes", 1, 1), p("cola", 1, 2), p("wasser", 1, 2)])
    expect(positionOptions(position!).map((o) => o.id)).toEqual(["pommes", "cola", "wasser"])
  })

  it("returns nothing for an empty campaign", () => {
    expect(campaignPositions([])).toEqual([])
  })

  it("keeps products apart when the response carries no choiceGroup", () => {
    const legacy = [{ id: "pommes" }, { id: "cola" }] as unknown as TestProduct[]
    expect(shape(legacy)).toEqual([[["pommes"]], [["cola"]]])
  })
})
