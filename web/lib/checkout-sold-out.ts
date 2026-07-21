// Shared hand-off for the shop's out-of-stock UX: the payment step (/checkout/payment)
// detects a sold-out result from the backend, stashes the offending items, and
// bounces to the cart (/checkout) where the exact lines are highlighted red.

export const SOLD_OUT_STORAGE_KEY = "comanda.sold_out"

export interface SoldOutItem {
  productId: string
  variantId?: string
  name?: string
  available?: number
  requested?: number
  // Deactivated rather than out of stock — changes the highlighted message.
  unavailable?: boolean
}

export function soldOutKey(productId: string, variantId?: string | null): string {
  return `${productId}::${variantId ?? ""}`
}

export function soldOutMessage(available?: number, unavailable?: boolean): string {
  if (unavailable) return "Nicht mehr verfügbar"
  return available && available > 0 ? `Nur noch ${available} verfügbar` : "Ausverkauft"
}

export type SoldOutMap = Map<string, SoldOutItem>

// buildSoldOutMap keys sold-out items by product+variant, for highlighting the
// exact cart lines.
export function buildSoldOutMap(items: SoldOutItem[]): SoldOutMap {
  const map: SoldOutMap = new Map()
  for (const it of items) map.set(soldOutKey(it.productId, it.variantId), it)
  return map
}

// Resolves the sold-out error for a cart line. A line matches on its own
// product+variant; a bundle line also matches when any configured component is
// short — prefixed with the component's name (already variant-qualified by the
// backend), since the line itself only names the bundle.
export function soldOutErrorForItem(
  map: SoldOutMap,
  item: {
    product: { id: string }
    variantId?: string
    configuration?: Record<string, string>
    configurationVariants?: Record<string, { id: string }>
  }
): string | undefined {
  const direct = map.get(soldOutKey(item.product.id, item.variantId))
  if (direct) return soldOutMessage(direct.available, direct.unavailable)
  if (!item.configuration) return undefined
  for (const [slotId, productId] of Object.entries(item.configuration)) {
    const hit = map.get(soldOutKey(productId, item.configurationVariants?.[slotId]?.id))
    if (hit) {
      const message = soldOutMessage(hit.available, hit.unavailable)
      return hit.name ? `${hit.name}: ${message}` : message
    }
  }
  return undefined
}

// extractSoldOutItems reads the `details.items` array off an out_of_stock ApiError.
export function extractSoldOutItems(details: unknown): SoldOutItem[] {
  if (!details || typeof details !== "object") return []
  const items = (details as { items?: unknown }).items
  if (!Array.isArray(items)) return []
  return items.filter((it): it is SoldOutItem => typeof it === "object" && it !== null && "productId" in it)
}

export function extractUnavailableItems(details: unknown): SoldOutItem[] {
  return extractSoldOutItems(details).map((it) => ({ ...it, unavailable: true }))
}

export function stashSoldOut(items: SoldOutItem[]): void {
  try {
    if (items.length) sessionStorage.setItem(SOLD_OUT_STORAGE_KEY, JSON.stringify(items))
  } catch {}
}

export function takeSoldOut(): SoldOutItem[] {
  try {
    const raw = sessionStorage.getItem(SOLD_OUT_STORAGE_KEY)
    if (!raw) return []
    sessionStorage.removeItem(SOLD_OUT_STORAGE_KEY)
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as SoldOutItem[]) : []
  } catch {
    return []
  }
}
