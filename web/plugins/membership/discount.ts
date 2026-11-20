import type { CartItem } from "@/types/cart"

export interface MembershipDiscountItem {
  cartItemId: string
  productId: string
  productName: string
  freeQuantity: number
  unitPriceCents: number
}

export interface MembershipDiscount {
  items: MembershipDiscountItem[]
  freeQuantity: number
  totalDiscountCents: number
}

// The dearest eligible units go first, so a member's one free item is the
// burger and not the coffee next to it. The backend books the same units when
// the payment is recorded.
export function computeMembershipDiscount(
  cartItems: CartItem[],
  freeProductIds: string[],
  remaining: number
): MembershipDiscount | null {
  if (freeProductIds.length === 0 || remaining <= 0) return null

  const eligible = cartItems
    .filter((item) => freeProductIds.includes(item.product.id))
    .sort((a, b) => b.totalPriceCents - a.totalPriceCents)

  const items: MembershipDiscountItem[] = []
  let left = remaining
  let totalDiscountCents = 0

  for (const item of eligible) {
    if (left <= 0) break
    const freeQuantity = Math.min(item.quantity, left)
    left -= freeQuantity
    totalDiscountCents += freeQuantity * item.totalPriceCents
    items.push({
      cartItemId: item.id,
      productId: item.product.id,
      productName: item.product.name,
      freeQuantity,
      unitPriceCents: item.totalPriceCents,
    })
  }

  if (items.length === 0) return null
  return { items, freeQuantity: remaining - left, totalDiscountCents }
}
