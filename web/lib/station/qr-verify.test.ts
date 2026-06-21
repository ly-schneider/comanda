import * as ed from "@noble/ed25519"
import { describe, expect, it } from "vitest"
import { type QrPayload, verifyQrToken } from "./qr-verify"

// Importing qr-verify wires the sync SHA-512 into noble, so ed.keygen/sign work.

function b64url(bytes: Uint8Array): string {
  let bin = ""
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]!)
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

function concat(a: Uint8Array, b: Uint8Array): Uint8Array {
  const out = new Uint8Array(a.length + b.length)
  out.set(a, 0)
  out.set(b, a.length)
  return out
}

const keypair = ed.keygen()
const secretKey = keypair.secretKey
const publicKey = b64url(keypair.publicKey)

// Mirrors the backend's canonical JSON: object key order is the literal signed
// byte order (v,t,o,i,l), so a JS-built body is byte-identical to Go's output.
function sign(payload: QrPayload, sk: Uint8Array = secretKey): string {
  const body = new TextEncoder().encode(JSON.stringify(payload))
  const sig = ed.sign(body, sk)
  return b64url(concat(body, sig))
}

const validPayload: QrPayload = {
  v: 1,
  t: "tenant_seed1",
  o: "HS79U1yH7Zd3",
  i: 1_700_000_000,
  l: [{ p: "prod_pommes1", q: 2 }],
}

describe("verifyQrToken", () => {
  it("accepts a genuine token and returns the parsed payload", () => {
    const res = verifyQrToken(sign(validPayload), publicKey)
    expect(res).toEqual({ ok: true, payload: validPayload })
  })

  it("rejects a tampered body with bad-signature", () => {
    const body = new TextEncoder().encode(JSON.stringify(validPayload))
    const sig = ed.sign(body, secretKey)
    const tampered = new Uint8Array(body)
    const idx = tampered.length - 5 // flip a bit inside the order id
    tampered[idx] = (tampered[idx] ?? 0) ^ 0x01
    const res = verifyQrToken(b64url(concat(tampered, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("rejects a token signed by a different key", () => {
    const other = ed.keygen()
    const res = verifyQrToken(sign(validPayload, other.secretKey), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("rejects an unknown payload version even with a valid signature", () => {
    const res = verifyQrToken(sign({ ...validPayload, v: 2 }), publicKey)
    expect(res).toEqual({ ok: false, reason: "unsupported-version" })
  })

  it("rejects a validly-signed body that is not payload JSON", () => {
    const body = new TextEncoder().encode("not a payload")
    const sig = ed.sign(body, secretKey)
    const res = verifyQrToken(b64url(concat(body, sig)), publicKey)
    expect(res).toEqual({ ok: false, reason: "bad-payload" })
  })

  it("treats a body-less (signature-only) token as malformed", () => {
    const res = verifyQrToken(b64url(new Uint8Array(64)), publicKey)
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
    expect(verifyQrToken(sign(validPayload), b64url(new Uint8Array(16)))).toEqual({
      ok: false,
      reason: "bad-signature",
    })
  })

  it("preserves every line of a multi-product payload", () => {
    const multi: QrPayload = {
      ...validPayload,
      l: [
        { p: "prod_pommes1", q: 1 },
        { p: "prod_cola___1", q: 3 },
      ],
    }
    const res = verifyQrToken(sign(multi), publicKey)
    expect(res).toEqual({ ok: true, payload: multi })
  })
})
