export const ENTITY_ID_RE = /^[1-9A-HKMNP-Za-hkmnp-z_-]{12}$/

const CAMPAIGN_PREFIX = "CAMP:"
// Anchored to /o/<id> — a loose 12-char window also matches inside long subdomains.
const ORDER_URL_RE = /\/o\/([1-9A-HKMNP-Za-hkmnp-z_-]{12})\/?$/

export type ParsedScan = { kind: "order" | "campaign"; id: string }

export function parseScan(raw: string): ParsedScan | null {
  const trimmed = (raw ?? "").trim()
  if (!trimmed) return null

  if (trimmed.toUpperCase().startsWith(CAMPAIGN_PREFIX)) {
    const token = trimmed.slice(CAMPAIGN_PREFIX.length).trim()
    return ENTITY_ID_RE.test(token) ? { kind: "campaign", id: token } : null
  }

  const orderId = trimmed.match(ORDER_URL_RE)?.[1]
  if (orderId) return { kind: "order", id: orderId }

  return ENTITY_ID_RE.test(trimmed) ? { kind: "order", id: trimmed } : null
}
