import { describe, expect, it } from "vitest"
import { parseScan } from "./station-scan"

describe("parseScan", () => {
  it("parses a campaign payload", () => {
    expect(parseScan("CAMP:tkn_camp___1")).toEqual({ kind: "campaign", token: "tkn_camp___1" })
  })

  it("is case-insensitive on the campaign prefix only", () => {
    expect(parseScan("camp:tkn_camp___1")).toEqual({ kind: "campaign", token: "tkn_camp___1" })
  })

  it("rejects campaign payloads with invalid tokens", () => {
    expect(parseScan("CAMP:not-an-id")).toBeNull()
    expect(parseScan("CAMP:01937c7a-0000-7000-8000-000000000000")).toBeNull()
  })

  it("routes any non-campaign string to the order path as a signed-token candidate", () => {
    const token = "eyJ2IjoxfQ.signature-bytes_base64url"
    expect(parseScan(`  ${token} `)).toEqual({ kind: "order", token })
    // Validity (signature, version, tenant) is decided by offline verification,
    // not here — even an old-style URL or garbage becomes an order candidate.
    expect(parseScan("https://acme.comanda.leys.ch/o/HS79U1yH7Zd3")).toEqual({
      kind: "order",
      token: "https://acme.comanda.leys.ch/o/HS79U1yH7Zd3",
    })
  })

  it("rejects only empty / whitespace input", () => {
    expect(parseScan("")).toBeNull()
    expect(parseScan("   ")).toBeNull()
  })
})
