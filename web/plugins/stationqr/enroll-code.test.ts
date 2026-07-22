import { describe, expect, it } from "vitest"
import { extractEnrollToken } from "./enroll-code"

describe("extractEnrollToken", () => {
  it("passes through a raw token", () => {
    expect(extractEnrollToken("abc123_-XYZ")).toBe("abc123_-XYZ")
  })

  it("trims whitespace around a raw token", () => {
    expect(extractEnrollToken("  abc123  ")).toBe("abc123")
  })

  it("extracts the code param from an enrollment URL", () => {
    expect(extractEnrollToken("https://shop.example.ch/station?code=tok_1")).toBe("tok_1")
  })

  it("extracts code among other query params", () => {
    expect(extractEnrollToken("https://shop.example.ch/station?utm=x&code=tok_1")).toBe("tok_1")
  })

  it("rejects a URL without a code param", () => {
    expect(extractEnrollToken("https://shop.example.ch/station")).toBeNull()
  })

  it("rejects a URL with an empty code param", () => {
    expect(extractEnrollToken("https://shop.example.ch/station?code=")).toBeNull()
  })

  it("rejects empty input", () => {
    expect(extractEnrollToken("   ")).toBeNull()
  })
})
