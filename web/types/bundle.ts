import type { ProductSummaryDTO } from "./product"

/** Full bundle object returned by GET /v1/bundles */
export interface Bundle {
  id: string
  categoryId: string
  name: string
  image: string | null
  description: string | null
  priceCents: number
  isActive: boolean
  createdAt: string
  updatedAt: string
  slots: BundleSlot[]
}

export interface BundleSlot {
  id: string
  bundleProductId: string
  name: string
  sequence: number
  options: BundleSlotOption[]
}

export interface BundleSlotOption {
  bundleSlotId: string
  optionProductId: string
  optionProduct?: {
    id: string
    name: string
    priceCents: number
    image: string | null
  }
}

export interface BundleSlotDTO {
  id: string
  name: string
  sequence: number
  options: BundleSlotItemDTO[] | null
}

export interface BundleSlotItem {
  id: string
  bundleSlotId: string
  productId: string
}

export type BundleSlotItemDTO = ProductSummaryDTO

export interface BundleDTO {
  slots: BundleSlotDTO[] | null
}
