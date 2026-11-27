import type { ComponentType } from "react"
import artistVoucher from "./artist-voucher/manifest"
import membership from "./membership/manifest"
import type { PlatformPluginConfigProps, PluginSlotComponents } from "./slots"
import staffPerks from "./staff-perks/manifest"
import stationqr from "./stationqr/manifest"

export type { PluginSlotName } from "./slots"

export type PluginManifest = {
  key: string
  name?: string
  slots: PluginSlotComponents
}

export const registry: PluginManifest[] = [membership, stationqr, artistVoucher, staffPerks]

export function getPluginManifest(key: string): PluginManifest | undefined {
  return registry.find((manifest) => manifest.key === key)
}

export function getPlatformConfigComponent(key: string): ComponentType<PlatformPluginConfigProps> | undefined {
  return getPluginManifest(key)?.slots["platform.plugin.config"]
}
