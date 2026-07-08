import { bytesToBase64Url } from "@/lib/base64url"

export const ENTITY_ID_RE = /^[1-9A-HKMNP-Za-hkmnp-z_-]{12}$/

const CAMPAIGN_PREFIX = "CAMP:"

// A campaign claim redeems online (needs the server counter); anything else is an
// order pickup-token candidate, verified offline. A v2 QR is raw binary bytes
// (first byte 0x02) which we re-encode to base64url so the pipeline stays text.
export type ParsedScan = { kind: "campaign"; token: string } | { kind: "order"; token: string }

export function parseScan(text: string, bytes?: Uint8Array): ParsedScan | null {
  const trimmed = (text ?? "").trim()

  if (trimmed.toUpperCase().startsWith(CAMPAIGN_PREFIX)) {
    const token = trimmed.slice(CAMPAIGN_PREFIX.length).trim()
    return ENTITY_ID_RE.test(token) ? { kind: "campaign", token } : null
  }

  if (bytes && bytes.length > 0 && bytes[0] === 0x02) {
    return { kind: "order", token: bytesToBase64Url(bytes) }
  }

  if (!trimmed) return null
  return { kind: "order", token: trimmed }
}
