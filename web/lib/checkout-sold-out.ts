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

// buildSoldOutMap keys sold-out items by product+variant → localized message, for
// highlighting the exact cart lines.
export function buildSoldOutMap(items: SoldOutItem[]): Map<string, string> {
  const map = new Map<string, string>()
  for (const it of items) map.set(soldOutKey(it.productId, it.variantId), soldOutMessage(it.available, it.unavailable))
  return map
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
