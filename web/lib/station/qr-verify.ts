/**
 * Offline verification of the signed pickup-QR token.
 *
 * The wire format is a cross-language contract fixed by the backend signer
 * (`internal/qrsign`): a token is `base64url_nopad(body || sig)` where `body`
 * is the canonical payload JSON (fixed key order v,t,o,i,l) and `sig` is the
 * trailing 64-byte Ed25519 signature over those literal body bytes. The
 * signature MUST be checked over the decoded bytes — never a re-serialized
 * payload, which would drift from the signer's byte order.
 *
 * Verification is intentionally pure-JS (@noble/ed25519 + @noble/hashes), not
 * WebCrypto: Safari has no Ed25519 in SubtleCrypto, and the station verifies
 * fully offline with no server backstop, so this is the sole forgery barrier.
 */

import * as ed from "@noble/ed25519"
import { sha512 } from "@noble/hashes/sha2.js"

// noble v3 only exposes the synchronous verify once a sync SHA-512 is wired in.
if (!ed.hashes.sha512) ed.hashes.sha512 = sha512

export const QR_PAYLOAD_VERSION = 1

const SIGNATURE_SIZE = 64
const PUBLIC_KEY_SIZE = 32

export interface QrLine {
  /** product id */
  p: string
  /** quantity (>= 1) */
  q: number
}

export interface QrPayload {
  /** payload schema version */
  v: number
  /** tenant id */
  t: string
  /** order id */
  o: string
  /**
   * issued-at, Unix seconds.
   *
   * Expiry is intentionally not enforced: the append-only per-station ledger
   * is the sole replay barrier. One scanner per stand means a token can only
   * ever be redeemed once for a given (orderId, productId) pair — time-based
   * expiry would add clock-skew fragility with no security benefit here.
   */
  i: number
  /** redeemable line items */
  l: QrLine[]
}

export type VerifyFailure = "malformed" | "bad-signature" | "bad-payload" | "unsupported-version"

export type VerifyResult = { ok: true; payload: QrPayload } | { ok: false; reason: VerifyFailure }

function base64UrlToBytes(s: string): Uint8Array | null {
  let std = s.replace(/-/g, "+").replace(/_/g, "/")
  const pad = std.length % 4
  if (pad === 1) return null // never a valid base64 length
  if (pad) std += "=".repeat(4 - pad)
  try {
    const binary = atob(std)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    return bytes
  } catch {
    return null
  }
}

function parsePayload(body: Uint8Array): QrPayload | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(new TextDecoder().decode(body))
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== "object") return null
  const p = parsed as Record<string, unknown>
  if (typeof p.v !== "number" || typeof p.t !== "string" || typeof p.o !== "string" || typeof p.i !== "number") {
    return null
  }
  if (!Array.isArray(p.l)) return null
  const lines: QrLine[] = []
  for (const raw of p.l) {
    if (!raw || typeof raw !== "object") return null
    const line = raw as Record<string, unknown>
    if (typeof line.p !== "string" || typeof line.q !== "number") return null
    lines.push({ p: line.p, q: line.q })
  }
  return { v: p.v, t: p.t, o: p.o, i: p.i, l: lines }
}

/**
 * Verifies a scanned token against the tenant's public key (base64url, 32 raw
 * bytes — as published at `GET /v1/qr-config`). Returns the parsed payload only
 * when the signature is valid AND the schema version is understood. A valid
 * signature alone is never enough: the caller must still enforce tenant scoping.
 */
export function verifyQrToken(token: string, publicKeyB64Url: string | null | undefined): VerifyResult {
  if (!publicKeyB64Url) return { ok: false, reason: "bad-signature" }
  const pub = base64UrlToBytes(publicKeyB64Url)
  if (!pub || pub.length !== PUBLIC_KEY_SIZE) return { ok: false, reason: "bad-signature" }

  const raw = base64UrlToBytes(token.trim())
  if (!raw || raw.length <= SIGNATURE_SIZE) return { ok: false, reason: "malformed" }

  const split = raw.length - SIGNATURE_SIZE
  const body = raw.subarray(0, split)
  const sig = raw.subarray(split)

  let valid: boolean
  try {
    valid = ed.verify(sig, body, pub)
  } catch {
    // noble throws on structurally invalid points/signatures — same outcome.
    return { ok: false, reason: "bad-signature" }
  }
  if (!valid) return { ok: false, reason: "bad-signature" }

  const payload = parsePayload(body)
  if (!payload) return { ok: false, reason: "bad-payload" }
  if (payload.v !== QR_PAYLOAD_VERSION) return { ok: false, reason: "unsupported-version" }
  return { ok: true, payload }
}
