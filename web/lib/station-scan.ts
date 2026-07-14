import { bytesToBase64Url } from "@/lib/base64url"
import { VER_V3, VER_VOUCHER } from "@/lib/station/qr-verify"

export const ENTITY_ID_RE = /^[1-9A-HKMNP-Za-hkmnp-z_-]{12}$/

const CAMPAIGN_PREFIX = "CAMP:"

// A campaign claim redeems online (needs the server counter); anything else is
// verified offline. Both offline token families are raw binary bytes we re-encode
// to base64url so the pipeline stays text: an order pickup QR (first byte 0x03)
// and an artist-voucher QR (first byte 0x10).
export type ParsedScan =
  { kind: "campaign"; token: string } | { kind: "order"; token: string } | { kind: "voucher"; token: string }

export function parseScan(text: string, bytes?: Uint8Array): ParsedScan | null {
  const trimmed = (text ?? "").trim()

  if (trimmed.toUpperCase().startsWith(CAMPAIGN_PREFIX)) {
    const token = trimmed.slice(CAMPAIGN_PREFIX.length).trim()
    return ENTITY_ID_RE.test(token) ? { kind: "campaign", token } : null
  }

  if (bytes && bytes.length > 0) {
    if (bytes[0] === VER_VOUCHER) return { kind: "voucher", token: bytesToBase64Url(bytes) }
    if (bytes[0] === VER_V3) return { kind: "order", token: bytesToBase64Url(bytes) }
  }

  if (!trimmed) return null
  return { kind: "order", token: trimmed }
}
