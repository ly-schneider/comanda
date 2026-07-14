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

  it("re-encodes a v3 binary order QR (first byte 0x03) to a base64url token", () => {
    const body = new Uint8Array([0x03, 1, 2, 3, 4, 5])
    // zxing hands the raw bytes as `.bytes`; `.text` is lossy mojibake and unused here.
    const parsed = parseScan("", body)
    expect(parsed).toEqual({ kind: "order", token: bytesToBase64Url(body) })
  })

  it("re-encodes an artist-voucher QR (first byte 0x10) to a base64url voucher token", () => {
    const body = new Uint8Array([0x10, 9, 8, 7, 6])
    expect(parseScan("", body)).toEqual({ kind: "voucher", token: bytesToBase64Url(body) })
  })

  it("carries a text order candidate (e.g. the pickup-URL fallback) verbatim", () => {
    const token = "some-non-binary-text_candidate"
    expect(parseScan(`  ${token} `, ascii(token))).toEqual({ kind: "order", token })
    // Validity (signature, version, tenant) is decided by offline verification,
    // not here — even the plain pickup-URL fallback becomes an order candidate.
    const url = "https://acme.comanda.leys.ch/o/HS79U1yH7Zd3"
    expect(parseScan(url, ascii(url))).toEqual({ kind: "order", token: url })
  })

  it("rejects only empty / whitespace input with no bytes", () => {
    expect(parseScan("")).toBeNull()
    expect(parseScan("   ")).toBeNull()
  })
})
