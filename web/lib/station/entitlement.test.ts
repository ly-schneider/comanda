import * as ed from "@noble/ed25519"
import { describe, expect, it } from "vitest"
import { base64UrlToBytes, bytesToBase64Url } from "@/lib/base64url"
import { parseScan } from "@/lib/station-scan"
import { encodeEntitlement, signEntitlement } from "./qr-test-helpers"
import { type EntitlementPayload, verifyEntitlementToken, verifyQrToken, verifyVoucherToken } from "./qr-verify"

// Canonical golden vector — byte-identical to backend/internal/qrsign
// (TestEntitlementGoldenVector). Seed = 0x00..0x1f, discriminator 0x11.
const GOLDEN = {
  seedHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
  pubB64Url: "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg",
  token:
    "EXRlbmFudF9zZWVkMUVudDdBYjJDZDlFZmVT8QBlVUKADlN0cmFzc2VuYWt0aW9uA3Byb2RfcG9tbWVzMQEAcHJvZF9idXJnZXIxAQFwcm9kX2NoZWVzZTEBAdPsxohp6QXgYcD6rGBQ6Kx6EcFufqcWSgdetV4pzr-mLD_ZBu7yXd_9uWl8ZTJsVGY_99TamduToPpXOjFAfgI",
  bodyHex:
    "1174656e616e745f7365656431456e743741623243643945666553f100655542800e537472617373656e616b74696f6e0370726f645f706f6d6d657331010070726f645f62757267657231010170726f645f636865657365310101",
}

const goldenPayload: EntitlementPayload = {
  tenantId: "tenant_seed1",
  voucherId: "Ent7Ab2Cd9Ef",
  validFrom: 1_700_000_000,
  validUntil: 1_700_086_400,
  label: "Strassenaktion",
  lines: [
    { productId: "prod_pommes1", quantity: 1, choiceGroup: 0 },
    { productId: "prod_burger1", quantity: 1, choiceGroup: 1 },
    { productId: "prod_cheese1", quantity: 1, choiceGroup: 1 },
  ],
}

function hexToBytes(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2)
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16)
  return out
}

function toHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("")
}

const seed = hexToBytes(GOLDEN.seedHex)
const publicKey = bytesToBase64Url(ed.getPublicKey(seed))
const sign = (p: EntitlementPayload, sk: Uint8Array = seed) => signEntitlement(p, sk)

describe("verifyEntitlementToken", () => {
  it("encodes the golden payload to the exact bytes the Go signer produces", () => {
    expect(toHex(encodeEntitlement(goldenPayload))).toBe(GOLDEN.bodyHex)
  })

  it("re-signs the golden payload to the identical token (deterministic, cross-language)", () => {
    expect(sign(goldenPayload)).toBe(GOLDEN.token)
  })

  it("verifies the byte-exact golden token minted by the Go signer", () => {
    expect(verifyEntitlementToken(GOLDEN.token, GOLDEN.pubB64Url)).toEqual({ ok: true, payload: goldenPayload })
  })

  it("derives the golden public key from the shared seed", () => {
    expect(publicKey).toBe(GOLDEN.pubB64Url)
  })

  it("rejects a token signed by another key", () => {
    const other = new Uint8Array(32).fill(9)
    expect(verifyEntitlementToken(sign(goldenPayload, other), publicKey)).toEqual({
      ok: false,
      reason: "bad-signature",
    })
  })

  it("rejects a tampered quantity", () => {
    const raw = base64UrlToBytes(GOLDEN.token)!
    raw[raw.length - 66] = 9
    expect(verifyEntitlementToken(bytesToBase64Url(raw), publicKey)).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("rejects garbage and truncated input", () => {
    expect(verifyEntitlementToken("!!!not base64!!!", publicKey)).toEqual({ ok: false, reason: "malformed" })
    expect(verifyEntitlementToken(GOLDEN.token.slice(0, 20), publicKey)).toEqual({ ok: false, reason: "malformed" })
  })

  it("rejects a missing or malformed public key", () => {
    expect(verifyEntitlementToken(GOLDEN.token, null)).toEqual({ ok: false, reason: "bad-signature" })
    expect(verifyEntitlementToken(GOLDEN.token, "short")).toEqual({ ok: false, reason: "bad-signature" })
  })

  it("round-trips a single-line slip", () => {
    const single: EntitlementPayload = {
      ...goldenPayload,
      lines: [{ productId: "prod_burger1", quantity: 1, choiceGroup: 0 }],
    }
    expect(verifyEntitlementToken(sign(single), publicKey)).toEqual({ ok: true, payload: single })
  })

  it("round-trips a multibyte label", () => {
    const umlaut: EntitlementPayload = { ...goldenPayload, label: "Grüezi Aktion – Süssgetränk" }
    expect(verifyEntitlementToken(sign(umlaut), publicKey)).toEqual({ ok: true, payload: umlaut })
  })

  it("rejects a body with trailing bytes the declared lengths don't cover", () => {
    const body = encodeEntitlement(goldenPayload)
    const padded = new Uint8Array(body.length + 1)
    padded.set(body)
    const sig = ed.sign(padded, seed)
    const token = new Uint8Array(padded.length + sig.length)
    token.set(padded)
    token.set(sig, padded.length)
    expect(verifyEntitlementToken(bytesToBase64Url(token), publicKey)).toEqual({ ok: false, reason: "bad-payload" })
  })
})

describe("discriminator isolation", () => {
  it("refuses to read an entitlement token as a pass or a pickup", () => {
    expect(verifyVoucherToken(GOLDEN.token, publicKey)).toEqual({ ok: false, reason: "unsupported-version" })
    expect(verifyQrToken(GOLDEN.token, publicKey)).toEqual({ ok: false, reason: "unsupported-version" })
  })

  it("routes the raw 0x11 bytes to the entitlement branch", () => {
    const bytes = base64UrlToBytes(GOLDEN.token)!
    expect(parseScan("", bytes)).toEqual({ kind: "entitlement", token: GOLDEN.token })
  })
})
