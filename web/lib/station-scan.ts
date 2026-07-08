import { bytesToBase64Url } from "@/lib/base64url"

export const ENTITY_ID_RE = /^[1-9A-HKMNP-Za-hkmnp-z_-]{12}$/

const CAMPAIGN_PREFIX = "CAMP:"

// `campaign` claims still redeem online (they need the server's redemption
// counter). `order` carries the self-contained signed pickup token verbatim —
// offline Ed25519 verification decides whether it is genuine, so anything that
// isn't a campaign claim is routed here as an order-token candidate.
//
// The token is always base64url text: a v2 pickup QR is raw binary bytes (first
// byte 0x02), which we re-encode to base64url here so the rest of the pipeline
// (offline verifier, backend lookup, dedup) stays string-based and symmetric
// with the Go verifier. The pickup-URL fallback QR is already text and is
// carried verbatim (offline verification then rejects it as not a token).
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
