"use client"

import type { ComponentType } from "react"
import { getPluginManifest } from "@/plugins/registry"
import type { PluginSlotCtx, SlotComponentProps } from "@/plugins/slots"
import { usePlugins } from "./plugin-provider"

export function PluginSlot<Name extends keyof PluginSlotCtx>({ name, ctx }: { name: Name; ctx: PluginSlotCtx[Name] }) {
  const { enabledPlugins } = usePlugins()

  const nodes = enabledPlugins.flatMap((plugin) => {
    const Component = getPluginManifest(plugin.key)?.slots[name] as ComponentType<SlotComponentProps<Name>> | undefined
    if (!Component) return []
    return [<Component key={plugin.key} ctx={ctx} config={plugin.config} />]
  })

  return <>{nodes}</>
}
