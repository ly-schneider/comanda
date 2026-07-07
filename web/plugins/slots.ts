import type { ComponentType } from "react"
import type { CartItem } from "@/types/cart"
import type { TenantPluginUpdate } from "@/types/plugin"

export type PluginSlotName =
  | "pos.basket.actions"
  | "admin.settings.section"
  | "device.pairing.actions"
  | "platform.plugin.config"

export type PluginDiscount = {
  pluginKey: string
  label: string
  amountCents: number
  paymentData: unknown
}

export type PosBasketActionsCtx = {
  cartItems: CartItem[]
  setPluginDiscount: (discount: PluginDiscount | null) => void
}

export type AdminSettingsSectionCtx = {
  token: string
}

export type DevicePairingActionsCtx = Record<string, never>

export type PlatformPluginConfigProps = {
  tenantId: string
  pluginKey: string
  config: Record<string, unknown>
  secretsConfiguredAt: string | null
  onSave: (patch: TenantPluginUpdate) => Promise<void>
  saving: boolean
}

export type PluginSlotCtx = {
  "pos.basket.actions": PosBasketActionsCtx
  "admin.settings.section": AdminSettingsSectionCtx
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
