import { cache } from "react"
import { apiRequest } from "@/lib/api"
import type { PublicPlugin, PublicPluginList } from "@/types/plugin"
import { getRequestHost } from "./host"

export const getPlugins = cache(async (): Promise<PublicPlugin[]> => {
  try {
    const host = await getRequestHost()
    const extraHeaders: Record<string, string> = {}
    if (host) extraHeaders["X-Forwarded-Host"] = host
    const data = await apiRequest<PublicPluginList>("/v1/plugins", { headers: extraHeaders })
    return data.items ?? []
  } catch {
    return []
  }
})
