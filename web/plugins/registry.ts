import type { ComponentType } from "react"
import artistVoucher from "./artist-voucher/manifest"
import club100 from "./club100/manifest"
import type { PlatformPluginConfigProps, PluginSlotComponents } from "./slots"
import stationqr from "./stationqr/manifest"

export type { PluginSlotName } from "./slots"

export type PluginManifest = {
  key: string
  name?: string
  slots: PluginSlotComponents
}

export const registry: PluginManifest[] = [club100, stationqr, artistVoucher]

export function getPluginManifest(key: string): PluginManifest | undefined {
  return registry.find((manifest) => manifest.key === key)
}

export function getPlatformConfigComponent(key: string): ComponentType<PlatformPluginConfigProps> | undefined {
  return getPluginManifest(key)?.slots["platform.plugin.config"]
}
