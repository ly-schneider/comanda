// Entity ids are 12-char nanoids from a 55-char alphabet (see backend
// internal/id). Mirrors of that contract live wherever a scanned payload must
// be parsed back into an id.
export const ENTITY_ID_RE = /^[1-9A-HKMNP-Za-hkmnp-z_-]{12}$/

const CAMPAIGN_PREFIX = "CAMP:"
// Order QR codes encode a pickup URL: `${origin}/o/<orderId>`. Extract the id
// from the path segment — a bare 12-char window match against the whole URL
// would also match inside long tenant subdomains.
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
