import type { Cents } from "./common"
import type { ProductDTO } from "./index"

export interface CartItemConfiguration {
  [slotId: string]: string // slotId -> selected productId
}

// slotId -> selected variant of the slot's product; value is snapshotted for
// display. Price adjustments never apply inside a bundle.
export interface CartItemConfigurationVariants {
  [slotId: string]: { id: string; value: string }
}

export interface CartItem {
  id: string
  product: ProductDTO
  quantity: number
  configuration?: CartItemConfiguration
  configurationVariants?: CartItemConfigurationVariants
  variantId?: string
  totalPriceCents: Cents
}

export interface Cart {
  items: CartItem[]
  totalCents: Cents
}

export interface CartContextType {
  cart: Cart
  addToCart: (
    product: ProductDTO,
    configuration?: CartItemConfiguration,
    variantId?: string,
    configurationVariants?: CartItemConfigurationVariants
  ) => void
  removeFromCart: (itemId: string) => void
  updateQuantity: (itemId: string, quantity: number) => void
  updateItemConfiguration: (
    oldItemId: string,
    product: ProductDTO,
    configuration?: CartItemConfiguration,
    configurationVariants?: CartItemConfigurationVariants
  ) => void
  clearCart: () => void
  getItemQuantity: (productId: string, configuration?: CartItemConfiguration, variantId?: string) => number
  getTotalProductQuantity: (productId: string) => number
}
