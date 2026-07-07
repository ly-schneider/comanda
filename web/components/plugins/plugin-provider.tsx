"use client"

import { createContext, useCallback, useContext, useMemo } from "react"
import type { PublicPlugin } from "@/types/plugin"

type PluginContextValue = {
  enabledPlugins: PublicPlugin[]
  getPluginConfig: (key: string) => Record<string, unknown> | undefined
}

const PluginContext = createContext<PluginContextValue>({
  enabledPlugins: [],
  getPluginConfig: () => undefined,
})

export function PluginProvider({
  enabledPlugins,
  children,
}: {
  enabledPlugins: PublicPlugin[]
  children: React.ReactNode
}) {
  const getPluginConfig = useCallback(
    (key: string) => enabledPlugins.find((plugin) => plugin.key === key)?.config,
    [enabledPlugins]
  )
  const value = useMemo(() => ({ enabledPlugins, getPluginConfig }), [enabledPlugins, getPluginConfig])
  return <PluginContext.Provider value={value}>{children}</PluginContext.Provider>
}

export function usePlugins(): PluginContextValue {
  return useContext(PluginContext)
}
