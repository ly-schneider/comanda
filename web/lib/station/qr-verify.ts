/**
 * Offline verification of the signed pickup-QR token.
 *
 * The wire format is a cross-language contract fixed by the backend signer
 * (`internal/qrsign`, see doc.go). A token is `base64url_nopad(body || sig)`
 * where `sig` is the trailing 64-byte Ed25519 signature over the literal `body`
 * bytes. After a successful signature check the body is parsed by its leading
 * byte: `0x02` = v2 fixed binary (current), `{` (0x7B) = legacy v1 JSON. The
 * signature MUST be checked over the decoded bytes — never a re-serialized
 * payload, which would drift from the signer's byte order.
 *
 * This verifier is symmetric with the Go verifier: both take the base64url
 * token text, decode once, then discriminate on the first body byte. The
 * station scanner reads a v2 QR as raw bytes and re-encodes them to base64url at
 * the scan boundary, so the token reaching here is always text.
 *
 * Verification is intentionally pure-JS (@noble/ed25519 + @noble/hashes), not
 * WebCrypto: Safari has no Ed25519 in SubtleCrypto, and the station verifies
 * fully offline with no server backstop, so this is the sole forgery barrier.
 */

import * as ed from "@noble/ed25519"
import { sha512 } from "@noble/hashes/sha2.js"
import { base64UrlToBytes } from "@/lib/base64url"

// noble v3 only exposes the synchronous verify once a sync SHA-512 is wired in.
if (!ed.hashes.sha512) ed.hashes.sha512 = sha512

// Current emit version. v1 is still accepted by the verifier during rollout.
export const QR_PAYLOAD_VERSION = 2

const SIGNATURE_SIZE = 64
const PUBLIC_KEY_SIZE = 32

// v2 binary layout (see qrsign/doc.go).
const VER_V2 = 0x02
const VER_V1 = 0x7b // '{' — a v1 body is canonical JSON
const ID_LEN = 12
const HEADER_LEN = 1 + ID_LEN + ID_LEN + 4 + 1 // 30
const LINE_LEN = ID_LEN + 1 // 13

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

function asciiOf(bytes: Uint8Array): string {
  let s = ""
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]!)
  return s
}

function parseV2(body: Uint8Array): QrPayload | null {
  if (body.length < HEADER_LEN) return null
  const count = body[HEADER_LEN - 1]!
  if (body.length !== HEADER_LEN + count * LINE_LEN) return null
  const t = asciiOf(body.subarray(1, 1 + ID_LEN))
  const o = asciiOf(body.subarray(1 + ID_LEN, 1 + 2 * ID_LEN))
  // uint32 big-endian; `>>> 0` keeps it unsigned.
  const i = ((body[25]! << 24) | (body[26]! << 16) | (body[27]! << 8) | body[28]!) >>> 0
  const l: QrLine[] = []
  let off = HEADER_LEN
  for (let k = 0; k < count; k++) {
    l.push({ p: asciiOf(body.subarray(off, off + ID_LEN)), q: body[off + ID_LEN]! })
    off += LINE_LEN
  }
  return { v: 2, t, o, i, l }
}

function parseV1(body: Uint8Array): QrPayload | null {
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
  if (p.v !== 1) return null
  if (!Array.isArray(p.l)) return null
  const lines: QrLine[] = []
  for (const raw of p.l) {
    if (!raw || typeof raw !== "object") return null
    const line = raw as Record<string, unknown>
    if (typeof line.p !== "string" || typeof line.q !== "number") return null
    lines.push({ p: line.p, q: line.q })
  }
  return { v: 1, t: p.t, o: p.o, i: p.i, l: lines }
}

/**
 * Verifies a scanned token (base64url text) against the tenant's public key
 * (base64url, 32 raw bytes — as published at `GET /v1/qr-config`). Returns the
 * parsed payload only when the signature is valid AND the body format is
 * understood. A valid signature alone is never enough: the caller must still
 * enforce tenant scoping.
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

  const first = body[0]
  if (first === VER_V2) {
    const payload = parseV2(body)
    return payload ? { ok: true, payload } : { ok: false, reason: "bad-payload" }
  }
  if (first === VER_V1) {
    const payload = parseV1(body)
    if (!payload) return { ok: false, reason: "bad-payload" }
    return { ok: true, payload }
  }
  return { ok: false, reason: "unsupported-version" }
}
