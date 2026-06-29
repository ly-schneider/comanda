import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"
import type { CartItem } from "@/types/cart"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// Sum of returnable-container deposits across a cart. The deposit is charged on
// the ordered product itself (per unit), mirroring the backend order-build path
// where menu components never add a deposit.
export function cartDepositCents(items: CartItem[]): number {
  return items.reduce((sum, item) => sum + (item.product.depositCents ?? 0) * item.quantity, 0)
}

// Formats a price in cents to Swiss Francs.
// Examples:
//  - 500  -> "CHF 5.-"
//  - 550  -> "CHF 5.50"
export function formatChf(cents: number): string {
  if (isNaN(cents)) return "CHF 0.-"
  const isWholeFranc = cents % 100 === 0
  if (isWholeFranc) {
    return `CHF ${Math.round(cents / 100)}.-`
  }
  return `CHF ${(cents / 100).toFixed(2)}`
}
