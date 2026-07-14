import * as ed from "@noble/ed25519"
import { describe, expect, it } from "vitest"
import { bytesToBase64Url } from "@/lib/base64url"
import { encodeV3, signV3 } from "./qr-test-helpers"
import { type QrPayload, verifyQrToken } from "./qr-verify"

// Importing qr-verify wires the sync SHA-512 into noble, so ed.getPublicKey/sign work.

// Canonical golden vectors — byte-identical to backend/internal/qrsign
// (TestGoldenVector). Seed = 0x00..0x1f.
const GOLDEN = {
  seedHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
  pubB64Url: "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg",
  token:
    "A3RlbmFudF9zZWVkMUhTNzlVMXlIN1pkM2VT8QACcHJvZF9wb21tZXMxdnJudF9zaXplX3hsAnByb2RfY29sYTAwMQAAAAAAAAAAAAAAAAH-io89pj7lJ4n0DW50ZTd5NU-q3E4FX1v8VUvSkn9XVq1-1uea_qSnTtS8W4XlnIHsbk0yK6ZT94-xQy6RX6QF",
}

function hexToBytes(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2)
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16)
  return out
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((n, p) => n + p.length, 0)
  const out = new Uint8Array(total)
  let off = 0
  for (const p of parts) {
    out.set(p, off)
    off += p.length
  }
  return out
}

const seed = hexToBytes(GOLDEN.seedHex)
const publicKey = bytesToBase64Url(ed.getPublicKey(seed))

const sign = (p: QrPayload, sk: Uint8Array = seed) => signV3(p, sk)

const validPayload: QrPayload = {
  v: 3,
  t: "tenant_seed1",
  o: "HS79U1yH7Zd3",
  i: 1_700_000_000,
  l: [
    { p: "prod_pommes1", v: "vrnt_size_xl", q: 2 },
    { p: "prod_cola001", q: 1 },
  ],
}

describe("verifyQrToken", () => {
  it("derives the golden public key from the shared seed", () => {
    expect(publicKey).toBe(GOLDEN.pubB64Url)
  })

  it("verifies the byte-exact golden token minted by the Go signer", () => {
    const res = verifyQrToken(GOLDEN.token, GOLDEN.pubB64Url)
    expect(res).toEqual({ ok: true, payload: validPayload })
  })

  it("re-signs the golden payload to the identical token (deterministic, cross-language)", () => {
    expect(sign(validPayload)).toBe(GOLDEN.token)
  })

  it("accepts a genuine v3 token and returns the parsed payload", () => {
    const res = verifyQrToken(sign(validPayload), publicKey)
    expect(res).toEqual({ ok: true, payload: validPayload })
  })

  it("splits same-product lines by their variant id", () => {
    const sameProduct: QrPayload = {
      ...validPayload,
      l: [
        { p: "prod_shirt01", v: "vrnt_size_xl", q: 1 },
        { p: "prod_shirt01", v: "vrnt_size_m0", q: 1 },
      ],
    }
    const res = verifyQrToken(sign(sameProduct), publicKey)
    expect(res).toEqual({ ok: true, payload: sameProduct })
  })

  it("rejects a retired v1 JSON token as unsupported-version", () => {
    // A validly-signed v1 JSON body starts with '{' (0x7B), not 0x03.
    const body = new TextEncoder().encode(
      JSON.stringify({ v: 1, t: "tenant_seed1", o: "HS79U1yH7Zd3", i: 1_700_000_000, l: [{ p: "prod_pommes1", q: 2 }] })
    )
    const sig = ed.sign(body, seed)
    const res = verifyQrToken(bytesToBase64Url(concat(body, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "unsupported-version" })
  })

  it("rejects a retired v2 (variant-less) body as unsupported-version", () => {
    // v2 was the same fixed binary with a leading 0x02 and 13-byte lines.
    const body = new Uint8Array([0x02, 0xaa, 0xbb, 0xcc])
    const sig = ed.sign(body, seed)
    const res = verifyQrToken(bytesToBase64Url(concat(body, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "unsupported-version" })
  })

  it("rejects a tampered v3 body with bad-signature", () => {
    const body = encodeV3(validPayload)
    const sig = ed.sign(body, seed)
    const tampered = new Uint8Array(body)
    tampered[2] = (tampered[2] ?? 0) ^ 0x01 // flip a tenant-id byte
    const res = verifyQrToken(bytesToBase64Url(concat(tampered, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("rejects a token signed by a different key", () => {
    const other = ed.utils.randomSecretKey()
    const res = verifyQrToken(sign(validPayload, other), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("rejects an unknown body version even with a valid signature", () => {
    const body = new Uint8Array([0x09, 0xaa, 0xbb])
    const sig = ed.sign(body, seed)
    const res = verifyQrToken(bytesToBase64Url(concat(body, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "unsupported-version" })
  })

  it("rejects a v3 body whose declared line count mismatches its length", () => {
    const body = encodeV3(validPayload)
    body[29] = 9 // claim 9 lines but the buffer only holds 2
    const sig = ed.sign(body, seed)
    const res = verifyQrToken(bytesToBase64Url(concat(body, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-payload" })
  })

  it("treats a body-less (signature-only) token as malformed", () => {
    const res = verifyQrToken(bytesToBase64Url(new Uint8Array(64)), publicKey)
    expect(res).toEqual({ ok: false, reason: "malformed" })
  })

  it("treats non-base64 / empty input as malformed", () => {
    expect(verifyQrToken("", publicKey)).toEqual({ ok: false, reason: "malformed" })
    expect(verifyQrToken("https://acme.comanda.leys.ch/o/HS79U1yH7Zd3", publicKey)).toEqual({
      ok: false,
      reason: "malformed",
    })
  })

  it("rejects when the public key is missing or the wrong length", () => {
    expect(verifyQrToken(sign(validPayload), null)).toEqual({ ok: false, reason: "bad-signature" })
    expect(verifyQrToken(sign(validPayload), bytesToBase64Url(new Uint8Array(16)))).toEqual({
      ok: false,
      reason: "bad-signature",
    })
  })

  it("preserves every line of a multi-product payload", () => {
    const multi: QrPayload = {
      ...validPayload,
      l: [
        { p: "prod_pommes1", v: "vrnt_size_xl", q: 1 },
        { p: "prod_cola001", q: 3 },
        { p: "prod_wurst01", q: 5 },
      ],
    }
    const res = verifyQrToken(sign(multi), publicKey)
    expect(res).toEqual({ ok: true, payload: multi })
  })
})
