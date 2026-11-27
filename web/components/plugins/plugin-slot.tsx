"use client"

import type { ComponentType, ReactNode } from "react"
import { getPluginManifest } from "@/plugins/registry"
import type { PluginSlotCtx, SlotComponentProps } from "@/plugins/slots"
import { usePlugins } from "./plugin-provider"

export function PluginSlot<Name extends keyof PluginSlotCtx>({
  name,
  ctx,
  fallback,
}: {
  name: Name
  ctx: PluginSlotCtx[Name]
  // Rendered when no enabled plugin serves the slot — for a slot that owns a
  // whole page, where showing nothing would be a blank screen.
  fallback?: ReactNode
}) {
  const { enabledPlugins } = usePlugins()

  const nodes = enabledPlugins.flatMap((plugin) => {
    const Component = getPluginManifest(plugin.key)?.slots[name] as ComponentType<SlotComponentProps<Name>> | undefined
    if (!Component) return []
    return [<Component key={plugin.key} ctx={ctx} config={plugin.config} />]
  })

  if (nodes.length === 0) return <>{fallback ?? null}</>
  return <>{nodes}</>
}
