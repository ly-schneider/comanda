/**
 * Offline verification of the signed pickup-QR token — the sole forgery barrier
 * on the station's offline path. Wire format: `internal/qrsign` doc.go. Pure-JS
 * (@noble/ed25519), not WebCrypto — Safari has no Ed25519 in SubtleCrypto.
 */

import * as ed from "@noble/ed25519"
import { sha512 } from "@noble/hashes/sha2.js"
import { base64UrlToBytes } from "@/lib/base64url"

// noble v3 only exposes the synchronous verify once a sync SHA-512 is wired in.
if (!ed.hashes.sha512) ed.hashes.sha512 = sha512

export const QR_PAYLOAD_VERSION = 2

const SIGNATURE_SIZE = 64
const PUBLIC_KEY_SIZE = 32

// v2 binary layout (see qrsign/doc.go).
const VER_V2 = 0x02
const ID_LEN = 12
const HEADER_LEN = 1 + ID_LEN + ID_LEN + 4 + 1 // 30
const LINE_LEN = ID_LEN + 1 // 13

export interface QrLine {
  p: string // product id
  q: number // quantity (>= 1)
}

export interface QrPayload {
  v: number
  t: string // tenant id
  o: string // order id
  // Issued-at (Unix seconds). Expiry is deliberately NOT enforced: the
  // append-only per-station ledger is the sole replay barrier (one scanner per
  // stand → a token redeems at most once per order+product), so a time window
  // would only add clock-skew fragility.
  i: number
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

// A valid signature alone is never enough — the caller must still enforce tenant
// scoping (a valid sig only proves *some* tenant signed it).
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

  if (body[0] !== VER_V2) return { ok: false, reason: "unsupported-version" }
  const payload = parseV2(body)
  return payload ? { ok: true, payload } : { ok: false, reason: "bad-payload" }
}
