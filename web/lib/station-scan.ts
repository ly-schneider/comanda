export const ENTITY_ID_RE = /^[1-9A-HKMNP-Za-hkmnp-z_-]{12}$/

const CAMPAIGN_PREFIX = "CAMP:"

// `campaign` claims still redeem online (they need the server's redemption
// counter). `order` carries the self-contained signed pickup token verbatim —
// offline Ed25519 verification decides whether it is genuine, so anything that
// isn't a campaign claim is routed here as an order-token candidate.
export type ParsedScan = { kind: "campaign"; token: string } | { kind: "order"; token: string }

export function parseScan(raw: string): ParsedScan | null {
  const trimmed = (raw ?? "").trim()
  if (!trimmed) return null

  if (trimmed.toUpperCase().startsWith(CAMPAIGN_PREFIX)) {
    const token = trimmed.slice(CAMPAIGN_PREFIX.length).trim()
    return ENTITY_ID_RE.test(token) ? { kind: "campaign", token } : null
  }

  return { kind: "order", token: trimmed }
}
