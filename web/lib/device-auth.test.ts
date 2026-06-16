import { describe, expect, it } from "vitest"
import { isStationTokenUsableOffline } from "@/lib/device-auth"

describe("isStationTokenUsableOffline", () => {
  it("trusts the presence of a token (the credential is opaque, validated server-side)", () => {
    expect(isStationTokenUsableOffline("any-opaque-device-token")).toBe(true)
  })

  it("rejects a missing or empty token", () => {
    expect(isStationTokenUsableOffline(null)).toBe(false)
    expect(isStationTokenUsableOffline("")).toBe(false)
  })
})
