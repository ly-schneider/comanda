import { describe, expect, it } from "vitest"
import { parseChfToCents } from "./refund-format"

describe("parseChfToCents", () => {
  it("parses a dot-decimal amount to cents", () => {
    expect(parseChfToCents("12.50")).toBe(1250)
    expect(parseChfToCents("2")).toBe(200)
    expect(parseChfToCents("0")).toBe(0)
  })

  it("accepts a comma decimal separator", () => {
    expect(parseChfToCents("12,50")).toBe(1250)
  })

  it("rounds to the nearest cent", () => {
    expect(parseChfToCents("12.999")).toBe(1300)
  })

  it("returns null for empty or whitespace-only input", () => {
    expect(parseChfToCents("")).toBeNull()
    expect(parseChfToCents("   ")).toBeNull()
  })

  it("returns null for non-numeric or non-finite input", () => {
    expect(parseChfToCents("abc")).toBeNull()
    expect(parseChfToCents("Infinity")).toBeNull()
    expect(parseChfToCents("NaN")).toBeNull()
  })
})
