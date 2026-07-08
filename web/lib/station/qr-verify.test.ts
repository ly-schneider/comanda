import * as ed from "@noble/ed25519"
import { describe, expect, it } from "vitest"
import { bytesToBase64Url } from "@/lib/base64url"
import { type QrPayload, verifyQrToken } from "./qr-verify"

// Importing qr-verify wires the sync SHA-512 into noble, so ed.getPublicKey/sign work.

// Canonical golden vectors — byte-identical to backend/internal/qrsign
// (TestGoldenVector). Seed = 0x00..0x1f.
const GOLDEN = {
  seedHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
  pubB64Url: "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg",
  token:
    "AnRlbmFudF9zZWVkMUhTNzlVMXlIN1pkM2VT8QACcHJvZF9wb21tZXMxAnByb2RfY29sYTAwMQHaMq10Re5WTXw5EhrtgeLUbwYKDAj0e4J3qslI-xRrgyJcC60lyKkgDqs911g5tvZRWQnA2B2SGyhK1G_xckUE",
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

// Encode a v2 binary body (mirrors backend encodeV2) so we can sign fresh tokens.
function encodeV2(p: QrPayload): Uint8Array {
  const enc = new TextEncoder()
  const header = new Uint8Array(30)
  header[0] = 0x02
  header.set(enc.encode(p.t), 1)
  header.set(enc.encode(p.o), 13)
  header[25] = (p.i >>> 24) & 0xff
  header[26] = (p.i >>> 16) & 0xff
  header[27] = (p.i >>> 8) & 0xff
  header[28] = p.i & 0xff
  header[29] = p.l.length
  const lines = p.l.map((l) => {
    const row = new Uint8Array(13)
    row.set(enc.encode(l.p), 0)
    row[12] = l.q
    return row
  })
  return concat(header, ...lines)
}

function signV2(p: QrPayload, sk: Uint8Array = seed): string {
  const body = encodeV2(p)
  const sig = ed.sign(body, sk)
  return bytesToBase64Url(concat(body, sig))
}

const validPayload: QrPayload = {
  v: 2,
  t: "tenant_seed1",
  o: "HS79U1yH7Zd3",
  i: 1_700_000_000,
  l: [
    { p: "prod_pommes1", q: 2 },
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
    expect(signV2(validPayload)).toBe(GOLDEN.token)
  })

  it("accepts a genuine v2 token and returns the parsed payload", () => {
    const res = verifyQrToken(signV2(validPayload), publicKey)
    expect(res).toEqual({ ok: true, payload: validPayload })
  })

  it("rejects a retired v1 JSON token as unsupported-version", () => {
    // A validly-signed v1 JSON body starts with '{' (0x7B), not 0x02.
    const body = new TextEncoder().encode(
      JSON.stringify({ v: 1, t: "tenant_seed1", o: "HS79U1yH7Zd3", i: 1_700_000_000, l: [{ p: "prod_pommes1", q: 2 }] })
    )
    const sig = ed.sign(body, seed)
    const res = verifyQrToken(bytesToBase64Url(concat(body, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "unsupported-version" })
  })

  it("rejects a tampered v2 body with bad-signature", () => {
    const body = encodeV2(validPayload)
    const sig = ed.sign(body, seed)
    const tampered = new Uint8Array(body)
    tampered[2] = (tampered[2] ?? 0) ^ 0x01 // flip a tenant-id byte
    const res = verifyQrToken(bytesToBase64Url(concat(tampered, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("rejects a token signed by a different key", () => {
    const other = ed.utils.randomSecretKey()
    const res = verifyQrToken(signV2(validPayload, other), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("rejects an unknown body version even with a valid signature", () => {
    const body = new Uint8Array([0x09, 0xaa, 0xbb])
    const sig = ed.sign(body, seed)
    const res = verifyQrToken(bytesToBase64Url(concat(body, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "unsupported-version" })
  })

  it("rejects a v2 body whose declared line count mismatches its length", () => {
    const body = encodeV2(validPayload)
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
    expect(verifyQrToken(signV2(validPayload), null)).toEqual({ ok: false, reason: "bad-signature" })
    expect(verifyQrToken(signV2(validPayload), bytesToBase64Url(new Uint8Array(16)))).toEqual({
      ok: false,
      reason: "bad-signature",
    })
  })

  it("preserves every line of a multi-product payload", () => {
    const multi: QrPayload = {
      ...validPayload,
      l: [
        { p: "prod_pommes1", q: 1 },
        { p: "prod_cola001", q: 3 },
        { p: "prod_wurst01", q: 5 },
      ],
    }
    const res = verifyQrToken(signV2(multi), publicKey)
    expect(res).toEqual({ ok: true, payload: multi })
  })
})
