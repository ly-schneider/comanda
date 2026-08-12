import { describe, expect, it } from "vitest"
import { campaignPositions } from "@/lib/volunteer-choices"

const p = (id: string, choiceGroup: number) => ({ id, choiceGroup })

describe("campaignPositions", () => {
  it("gives every ungrouped product its own position", () => {
    const positions = campaignPositions([p("pommes", 0), p("getraenk", 0)])
    expect(positions.map((pos) => pos.options.map((o) => o.id))).toEqual([["pommes"], ["getraenk"]])
  })

  it("collapses a choice group into one position", () => {
    const positions = campaignPositions([p("wurst", 0), p("pommes", 1), p("getraenk", 1)])
    expect(positions.map((pos) => pos.options.map((o) => o.id))).toEqual([["wurst"], ["pommes", "getraenk"]])
  })

  it("keeps separate groups apart", () => {
    const positions = campaignPositions([p("pommes", 1), p("bier", 2), p("salat", 1), p("wasser", 2)])
    expect(positions.map((pos) => pos.choiceGroup)).toEqual([1, 2])
    expect(positions.map((pos) => pos.options.map((o) => o.id))).toEqual([
      ["pommes", "salat"],
      ["bier", "wasser"],
    ])
  })

  it("returns nothing for an empty campaign", () => {
    expect(campaignPositions([])).toEqual([])
  })

  it("keeps products apart when the response carries no choiceGroup", () => {
    const legacy = [{ id: "pommes" }, { id: "cola" }] as unknown as { id: string; choiceGroup: number }[]
    expect(campaignPositions(legacy).map((pos) => pos.options.map((o) => o.id))).toEqual([["pommes"], ["cola"]])
  })
})
