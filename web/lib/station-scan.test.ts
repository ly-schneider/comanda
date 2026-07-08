import { describe, expect, it } from "vitest"
import { bytesToBase64Url } from "@/lib/base64url"
import { parseScan } from "./station-scan"

const ascii = (s: string) => new TextEncoder().encode(s)

describe("parseScan", () => {
  it("parses a campaign payload (routes on text)", () => {
    expect(parseScan("CAMP:tkn_camp___1", ascii("CAMP:tkn_camp___1"))).toEqual({
      kind: "campaign",
      token: "tkn_camp___1",
    })
  })

  it("is case-insensitive on the campaign prefix only", () => {
    expect(parseScan("camp:tkn_camp___1", ascii("camp:tkn_camp___1"))).toEqual({
      kind: "campaign",
      token: "tkn_camp___1",
    })
  })

  it("rejects campaign payloads with invalid tokens", () => {
    expect(parseScan("CAMP:not-an-id")).toBeNull()
    expect(parseScan("CAMP:01937c7a-0000-7000-8000-000000000000")).toBeNull()
  })

  it("re-encodes a v2 binary order QR (first byte 0x02) to a base64url token", () => {
    const body = new Uint8Array([0x02, 1, 2, 3, 4, 5])
    // zxing hands the raw bytes as `.bytes`; `.text` is lossy mojibake and unused here.
    const parsed = parseScan("", body)
    expect(parsed).toEqual({ kind: "order", token: bytesToBase64Url(body) })
  })

  it("carries a legacy v1 / text order token verbatim", () => {
    const token = "eyJ2IjoxfQ.signature-bytes_base64url"
    expect(parseScan(`  ${token} `, ascii(token))).toEqual({ kind: "order", token })
    // Validity (signature, version, tenant) is decided by offline verification,
    // not here — even an old-style URL becomes an order candidate.
    const url = "https://acme.comanda.leys.ch/o/HS79U1yH7Zd3"
    expect(parseScan(url, ascii(url))).toEqual({ kind: "order", token: url })
  })

  it("rejects only empty / whitespace input with no bytes", () => {
    expect(parseScan("")).toBeNull()
    expect(parseScan("   ")).toBeNull()
  })
})
