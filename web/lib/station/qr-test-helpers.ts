/**
 * Shared token encoders for station tests — each mirrors its backend counterpart
 * byte-for-byte: v3 pickup (header 30 + n×(product 12 + variant 12 + qty 1)) and
 * the 0x11 entitlement voucher. Kept in one place so a wire-format change is a
 * single edit, cross-checked by the golden vectors in the verify tests.
 */

import * as ed from "@noble/ed25519"
import { sha512 } from "@noble/hashes/sha2.js"
import { bytesToBase64Url } from "@/lib/base64url"
import type { EntitlementPayload, QrPayload } from "./qr-verify"

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
  return attachSignature(body, sk)
}

export function encodeEntitlement(p: EntitlementPayload): Uint8Array {
  const enc = new TextEncoder()
  const label = enc.encode(p.label)
  const out = new Uint8Array(33 + 1 + label.length + 1 + p.lines.length * 14)
  out[0] = 0x11
  out.set(enc.encode(p.tenantId), 1)
  out.set(enc.encode(p.voucherId), 13)
  writeU32BE(out, 25, p.validFrom)
  writeU32BE(out, 29, p.validUntil)
  out[33] = label.length
  out.set(label, 34)

  let off = 34 + label.length
  out[off] = p.lines.length
  off += 1
  for (const line of p.lines) {
    out.set(enc.encode(line.productId), off)
    out[off + 12] = line.quantity
    out[off + 13] = line.choiceGroup
    off += 14
  }
  return out
}

export function signEntitlement(p: EntitlementPayload, sk: Uint8Array): string {
  return attachSignature(encodeEntitlement(p), sk)
}

function writeU32BE(out: Uint8Array, off: number, value: number): void {
  out[off] = (value >>> 24) & 0xff
  out[off + 1] = (value >>> 16) & 0xff
  out[off + 2] = (value >>> 8) & 0xff
  out[off + 3] = value & 0xff
}

function attachSignature(body: Uint8Array, sk: Uint8Array): string {
  const sig = ed.sign(body, sk)
  const out = new Uint8Array(body.length + sig.length)
  out.set(body, 0)
  out.set(sig, body.length)
  return bytesToBase64Url(out)
}
