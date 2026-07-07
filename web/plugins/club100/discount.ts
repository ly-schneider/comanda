import type { CartItem } from "@/types/cart"

export interface Club100DiscountItem {
  cartItemId: string
  productId: string
  productName: string
  quantity: number
  discountedQuantity: number
  unitPriceCents: number
}

export interface Club100Member {
  id: string
  firstName: string
  lastName: string
  remaining: number
  max: number
}

export interface Club100Discount {
  person: Club100Member
  discountedItems: Club100DiscountItem[]
  totalDiscountCents: number
  freeProductIds: string[]
}

export function computeClub100Discount(
  person: Club100Member,
  cartItems: CartItem[],
  freeProductIds: string[]
): Club100Discount | null {
  if (freeProductIds.length === 0 || person.remaining <= 0) {
    return null
  }

  const discountedItems: Club100DiscountItem[] = []
  let remainingRedemptions = person.remaining
  let totalDiscountCents = 0

  for (const item of cartItems) {
    if (remainingRedemptions <= 0) break
    if (!freeProductIds.includes(item.product.id)) continue

    const discountedQuantity = Math.min(item.quantity, remainingRedemptions)
    remainingRedemptions -= discountedQuantity
    totalDiscountCents += discountedQuantity * item.totalPriceCents

    discountedItems.push({
      cartItemId: item.id,
      productId: item.product.id,
      productName: item.product.name,
      quantity: item.quantity,
      discountedQuantity,
      unitPriceCents: item.totalPriceCents,
    })
  }

  if (discountedItems.length === 0) {
    return null
  }

  return {
    person: {
      id: person.id,
      firstName: person.firstName,
      lastName: person.lastName,
      remaining: person.remaining,
      max: person.max,
    },
    discountedItems,
    totalDiscountCents,
    freeProductIds,
  }
}
