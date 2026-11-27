import type { ComponentType } from "react"
import type { CartItem } from "@/types/cart"
import type { TenantPluginUpdate } from "@/types/plugin"

export type PluginSlotName =
  | "pos.gratis.actions"
  | "shop.home.actions"
  | "shop.claim.page"
  | "admin.settings.section"
  | "admin.stations.actions"
  | "device.pairing.actions"
  | "platform.plugin.config"

export type PluginDiscount = {
  pluginKey: string
  label: string
  amountCents: number
  paymentData: unknown
}

export type PosGratisActionsCtx = {
  cartItems: CartItem[]
  applyDiscount: (discount: PluginDiscount | null) => void
}

// Shop slots render for guests, who bring nothing with them: what a plugin
// needs there it fetches itself.
export type ShopHomeActionsCtx = Record<string, never>

export type ShopClaimPageCtx = Record<string, never>

export type AdminSettingsSectionCtx = {
  token: string
}

export type AdminStationsActionsCtx = {
  token: string
}

export type DevicePairingActionsCtx = {
  deviceType: "POS" | "STATION"
}

export type PlatformPluginConfigProps = {
  tenantId: string
  pluginKey: string
  config: Record<string, unknown>
  secretsConfiguredAt: string | null
  onSave: (patch: TenantPluginUpdate) => Promise<void>
  saving: boolean
}

export type PluginSlotCtx = {
  "pos.gratis.actions": PosGratisActionsCtx
  "shop.home.actions": ShopHomeActionsCtx
  "shop.claim.page": ShopClaimPageCtx
  "admin.settings.section": AdminSettingsSectionCtx
  "admin.stations.actions": AdminStationsActionsCtx
  "device.pairing.actions": DevicePairingActionsCtx
}

export type SlotComponentProps<Name extends keyof PluginSlotCtx> = {
  ctx: PluginSlotCtx[Name]
  config: Record<string, unknown>
}

export type PluginSlotComponents = {
  [Name in keyof PluginSlotCtx]?: ComponentType<SlotComponentProps<Name>>
} & {
  "platform.plugin.config"?: ComponentType<PlatformPluginConfigProps>
}
