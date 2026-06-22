import { cache } from "react"
import { apiRequest } from "@/lib/api"
import { getRequestHost } from "./host"

export type Branding = {
  displayName: string
  logoUrl: string | null
  faviconUrl: string | null
  colors: {
    primary: string
    primaryForeground: string
    secondary: string
    background: string
    foreground: string
  }
  fontKey: string
  radiusKey: string
  contact: {
    email: string | null
    phone: string | null
  }
  legal: {
    name: string | null
    address: string | null
    text: string | null
  }
  currency: string
}

const DEFAULTS: Branding = {
  displayName: "Comanda",
  logoUrl: null,
  faviconUrl: null,
  colors: {
    primary: "#0a0a0a",
    primaryForeground: "#ffffff",
    secondary: "#f5f5f5",
    background: "#ffffff",
    foreground: "#0a0a0a",
  },
  fontKey: "inter",
  radiusKey: "rounded",
  contact: { email: null, phone: null },
  legal: { name: null, address: null, text: null },
  currency: "CHF",
}

export const getBranding = cache(async (): Promise<Branding> => {
  try {
    const host = await getRequestHost()
    const extraHeaders: Record<string, string> = {}
    if (host) extraHeaders["X-Forwarded-Host"] = host
    const data = await apiRequest<Branding>("/v1/branding", { headers: extraHeaders })
    return { ...DEFAULTS, ...data, colors: { ...DEFAULTS.colors, ...data.colors } }
  } catch {
    return DEFAULTS
  }
})
