import { BundleDTO, BundleSlotDTO } from "@/types/bundle"
import { Cart, CartItem } from "@/types/cart"
import { ProductDTO } from "@/types/product"

export interface BundleSuggestion {
  bundleProduct: ProductDTO
  configuration: Record<string, string> // slotId -> productId
  sourceItems: { slotId: string; cartItem: CartItem }[]
  savingsCents: number
}

function canMapCartItemToSlot(cartItem: CartItem, slot: BundleSlotDTO): boolean {
  if (!slot.options) return false
  return slot.options.some((p) => p.id === cartItem.product.id)
}

function buildConfiguration(
  slots: BundleSlotDTO[],
  chosen: { slotId: string; cartItem: CartItem }[]
): Record<string, string> {
  const cfg: Record<string, string> = {}
  for (const { slotId, cartItem } of chosen) {
    cfg[slotId] = cartItem.product.id
  }
  for (const s of slots) {
    if (!(s.id in cfg)) cfg[s.id] = ""
  }
  return cfg
}

export function findBestBundleSuggestion(cart: Cart, allProducts: ProductDTO[]): BundleSuggestion | null {
  const bundles = allProducts.filter(
    (p) => p.type === "bundle" && p.bundle && p.bundle.slots && p.bundle.slots.length > 0
  )
  if (bundles.length === 0) return null

  const simpleItems = cart.items.filter((i) => i.product.type === "simple" && i.quantity > 0)
  if (simpleItems.length === 0) return null

  let best: BundleSuggestion | null = null

  for (const bundle of bundles) {
    const slots = (bundle.bundle as BundleDTO).slots as BundleSlotDTO[]

    const chosen: { slotId: string; cartItem: CartItem }[] = []
    const usedItemIds = new Set<string>()

    for (const slot of slots) {
      const match = simpleItems.find((it) => !usedItemIds.has(it.id) && canMapCartItemToSlot(it, slot))
      if (!match) {
        chosen.length = 0
        break
      }
      chosen.push({ slotId: slot.id, cartItem: match })
      usedItemIds.add(match.id)
    }

    if (chosen.length === slots.length) {
      const sumSimple = chosen.reduce((sum, ch) => sum + ch.cartItem.product.priceCents, 0)
      const savings = sumSimple - bundle.priceCents
      if (savings >= 0) {
        const suggestion: BundleSuggestion = {
          bundleProduct: bundle,
          configuration: buildConfiguration(slots, chosen),
          sourceItems: chosen,
          savingsCents: savings,
        }
        if (!best || suggestion.savingsCents > best.savingsCents) {
          best = suggestion
        }
      }
    }
  }

  return best
}
