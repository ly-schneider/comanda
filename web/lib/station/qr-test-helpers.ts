/**
 * Shared v3 pickup-token encoder for station tests — mirrors backend encodeV3
 * byte-for-byte (header 30 + n×(product 12 + variant 12 + qty 1)). Kept in one
 * place so a wire-format change is a single edit, cross-checked by the golden
 * vector in qr-verify.test.ts.
 */

import * as ed from "@noble/ed25519"
import { sha512 } from "@noble/hashes/sha2.js"
import { bytesToBase64Url } from "@/lib/base64url"
import type { QrPayload } from "./qr-verify"

// noble v3 only exposes the synchronous sign/verify once a sync SHA-512 is wired.
if (!ed.hashes.sha512) ed.hashes.sha512 = sha512

export function encodeV3(p: QrPayload): Uint8Array {
  const enc = new TextEncoder()
  const out = new Uint8Array(30 + p.l.length * 25)
  out[0] = 0x03
  out.set(enc.encode(p.t), 1)
  out.set(enc.encode(p.o), 13)
  out[25] = (p.i >>> 24) & 0xff
  out[26] = (p.i >>> 16) & 0xff
  out[27] = (p.i >>> 8) & 0xff
  out[28] = p.i & 0xff
  out[29] = p.l.length
  let off = 30
  for (const l of p.l) {
    out.set(enc.encode(l.p), off)
    if (l.v) out.set(enc.encode(l.v), off + 12)
    out[off + 24] = l.q
    off += 25
  }
  return out
}

export function signV3(p: QrPayload, sk: Uint8Array): string {
  const body = encodeV3(p)
  const sig = ed.sign(body, sk)
  const out = new Uint8Array(body.length + sig.length)
  out.set(body, 0)
  out.set(sig, body.length)
  return bytesToBase64Url(out)
}
