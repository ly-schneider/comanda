import * as ed from "@noble/ed25519"
import { describe, expect, it } from "vitest"
import { base64UrlToBytes, bytesToBase64Url } from "@/lib/base64url"
import { parseScan } from "@/lib/station-scan"
import { verifyVoucherToken, type VoucherPayload } from "./qr-verify"
import { evaluateVoucher } from "./voucher"

// Canonical golden vector — byte-identical to backend/internal/qrsign
// (TestVoucherGoldenVector). Seed = 0x00..0x1f, discriminator 0x10.
const GOLDEN = {
  seedHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
  pubB64Url: "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg",
  token:
    "EHRlbmFudF9zZWVkMVZjaDdBYjJDZDlFZmVT8QBlVUKACUxpdmUgQmFuZAIGRHJpbmtzBEZvb2S8kQe39D7m2BieSGN4fzWqDHyN18POcyqGizisVN_jU_bQ3r3EMF-OkMRl9WSPaAhWksbXZX7nOXKY_vgLgxYL",
}

const goldenPayload: VoucherPayload = {
  tenantId: "tenant_seed1",
  voucherId: "Vch7Ab2Cd9Ef",
  validFrom: 1_700_000_000,
  validUntil: 1_700_086_400,
  label: "Live Band",
  categories: ["Drinks", "Food"],
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
const enc = new TextEncoder()

// Mirrors backend encodeVoucher so tests can mint fresh tokens beyond the golden vector.
function encodeVoucher(p: VoucherPayload): Uint8Array {
  const header = new Uint8Array(1 + 12 + 12 + 4 + 4)
  header[0] = 0x10
  header.set(enc.encode(p.tenantId), 1)
  header.set(enc.encode(p.voucherId), 13)
  const dv = new DataView(header.buffer)
  dv.setUint32(25, p.validFrom, false)
  dv.setUint32(29, p.validUntil, false)
  const label = enc.encode(p.label)
  const cats = p.categories.map((c) => enc.encode(c))
  const parts: Uint8Array[] = [header, Uint8Array.of(label.length), label, Uint8Array.of(cats.length)]
  for (const c of cats) parts.push(Uint8Array.of(c.length), c)
  return concat(...parts)
}

function signVoucher(p: VoucherPayload, sk: Uint8Array = seed): string {
  const body = encodeVoucher(p)
  return bytesToBase64Url(concat(body, ed.sign(body, sk)))
}

const publicKey = bytesToBase64Url(ed.getPublicKey(seed))

describe("verifyVoucherToken", () => {
  it("derives the golden public key from the shared seed", () => {
    expect(publicKey).toBe(GOLDEN.pubB64Url)
  })

  it("verifies the byte-exact golden token minted by the Go signer", () => {
    expect(verifyVoucherToken(GOLDEN.token, GOLDEN.pubB64Url)).toEqual({ ok: true, payload: goldenPayload })
  })

  it("re-signs the golden payload to the identical token (deterministic, cross-language)", () => {
    expect(signVoucher(goldenPayload)).toBe(GOLDEN.token)
  })

  it("round-trips a UTF-8 label and category names", () => {
    const p: VoucherPayload = {
      tenantId: "tenant_abc12",
      voucherId: "voucher_xyz9",
      validFrom: 1_700_000_000,
      validUntil: 1_700_100_000,
      label: "Künstler ✦ Gruppe",
      categories: ["Getränke", "Süsses"],
    }
    expect(verifyVoucherToken(signVoucher(p), publicKey)).toEqual({ ok: true, payload: p })
  })

  it("round-trips a voucher with no categories", () => {
    const p: VoucherPayload = { ...goldenPayload, voucherId: "voucher_none", categories: [] }
    expect(verifyVoucherToken(signVoucher(p), publicKey)).toEqual({ ok: true, payload: p })
  })

  it("rejects a tampered body with bad-signature", () => {
    const body = encodeVoucher(goldenPayload)
    const tampered = new Uint8Array(body)
    tampered[2] = (tampered[2] ?? 0) ^ 0x01 // flip a tenant-id byte
    expect(verifyVoucherToken(bytesToBase64Url(concat(tampered, ed.sign(body, seed))), publicKey)).toEqual({
      ok: false,
      reason: "bad-signature",
    })
  })

  it("rejects a token signed by a different key", () => {
    const other = ed.utils.randomSecretKey()
    expect(verifyVoucherToken(signVoucher(goldenPayload, other), publicKey)).toEqual({
      ok: false,
      reason: "bad-signature",
    })
  })

  it("rejects a 0x02 pickup body as unsupported-version", () => {
    const body = new Uint8Array([0x02, 0xaa, 0xbb])
    expect(verifyVoucherToken(bytesToBase64Url(concat(body, ed.sign(body, seed))), publicKey)).toEqual({
      ok: false,
      reason: "unsupported-version",
    })
  })

  it("rejects a body whose declared category length overruns the buffer", () => {
    const body = encodeVoucher(goldenPayload)
    body[body.length - 5] = 200 // "Food" length byte → claims 200 bytes that don't exist
    expect(verifyVoucherToken(bytesToBase64Url(concat(body, ed.sign(body, seed))), publicKey)).toEqual({
      ok: false,
      reason: "bad-payload",
    })
  })

  it("treats non-base64 / signature-only input as malformed", () => {
    expect(verifyVoucherToken("", publicKey)).toEqual({ ok: false, reason: "malformed" })
    expect(verifyVoucherToken(bytesToBase64Url(new Uint8Array(64)), publicKey)).toEqual({
      ok: false,
      reason: "malformed",
    })
  })

  it("rejects a missing or wrong-length public key", () => {
    expect(verifyVoucherToken(GOLDEN.token, null)).toEqual({ ok: false, reason: "bad-signature" })
    expect(verifyVoucherToken(GOLDEN.token, bytesToBase64Url(new Uint8Array(16)))).toEqual({
      ok: false,
      reason: "bad-signature",
    })
  })
})

describe("evaluateVoucher", () => {
  const inWindow = 1_700_040_000_000 // ms, between valid_from and valid_until

  it("is valid inside the window for the right tenant", () => {
    expect(evaluateVoucher(GOLDEN.token, { publicKey, stationTenantId: "tenant_seed1", now: inWindow })).toEqual({
      kind: "valid",
      voucher: goldenPayload,
    })
  })

  it("valid_from is inclusive, valid_until is exclusive", () => {
    const ctx = { publicKey, stationTenantId: "tenant_seed1" }
    expect(evaluateVoucher(GOLDEN.token, { ...ctx, now: 1_700_000_000_000 }).kind).toBe("valid")
    expect(evaluateVoucher(GOLDEN.token, { ...ctx, now: 1_700_000_000_000 - 1 }).kind).toBe("scheduled")
    expect(evaluateVoucher(GOLDEN.token, { ...ctx, now: 1_700_086_400_000 }).kind).toBe("expired")
    expect(evaluateVoucher(GOLDEN.token, { ...ctx, now: 1_700_086_400_000 - 1 }).kind).toBe("valid")
  })

  it("is scheduled before valid_from", () => {
    expect(
      evaluateVoucher(GOLDEN.token, { publicKey, stationTenantId: "tenant_seed1", now: 1_600_000_000_000 })
    ).toEqual({ kind: "scheduled", voucher: goldenPayload })
  })

  it("is expired at/after valid_until", () => {
    expect(
      evaluateVoucher(GOLDEN.token, { publicKey, stationTenantId: "tenant_seed1", now: 1_800_000_000_000 })
    ).toEqual({ kind: "expired", voucher: goldenPayload })
  })

  it("rejects a voucher issued by a different tenant even with a valid signature", () => {
    expect(evaluateVoucher(GOLDEN.token, { publicKey, stationTenantId: "tenant_other", now: inWindow })).toEqual({
      kind: "invalid",
      reason: "wrong-tenant",
    })
  })

  it("is invalid (not-ready) when the verification material isn't cached", () => {
    expect(evaluateVoucher(GOLDEN.token, { publicKey: null, stationTenantId: "tenant_seed1", now: inWindow })).toEqual({
      kind: "invalid",
      reason: "not-ready",
    })
    expect(evaluateVoucher(GOLDEN.token, { publicKey, stationTenantId: null, now: inWindow })).toEqual({
      kind: "invalid",
      reason: "not-ready",
    })
  })

  it("surfaces a bad signature as invalid", () => {
    const other = ed.utils.randomSecretKey()
    expect(
      evaluateVoucher(signVoucher(goldenPayload, other), { publicKey, stationTenantId: "tenant_seed1", now: inWindow })
    ).toEqual({ kind: "invalid", reason: "bad-signature" })
  })

  it("reports revoked (from the predicate) ahead of the window", () => {
    const ctx = {
      publicKey,
      stationTenantId: "tenant_seed1",
      now: inWindow,
      revoked: (id: string) => id === "Vch7Ab2Cd9Ef",
    }
    expect(evaluateVoucher(GOLDEN.token, ctx)).toEqual({ kind: "revoked", voucher: goldenPayload })
  })

  it("ignores the revoked predicate for a different voucher id", () => {
    const ctx = {
      publicKey,
      stationTenantId: "tenant_seed1",
      now: inWindow,
      revoked: (id: string) => id === "some_other_id",
    }
    expect(evaluateVoucher(GOLDEN.token, ctx).kind).toBe("valid")
  })
})

// Raw QR bytes → parseScan → evaluateVoucher, end-to-end against the Go-minted bytes.
describe("scan pipeline (raw bytes → verdict)", () => {
  it("classifies the golden voucher bytes and renders a valid verdict", () => {
    const raw = base64UrlToBytes(GOLDEN.token)! // the exact bytes a 0x10 byte-mode QR carries
    expect(raw[0]).toBe(0x10)
    const parsed = parseScan("", raw)
    expect(parsed).toEqual({ kind: "voucher", token: GOLDEN.token })
    const verdict = evaluateVoucher(parsed!.token, {
      publicKey,
      stationTenantId: "tenant_seed1",
      now: 1_700_040_000_000,
    })
    expect(verdict).toEqual({ kind: "valid", voucher: goldenPayload })
  })
})
