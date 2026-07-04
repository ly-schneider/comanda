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
  legal: { name: null, address: null },
  currency: "CHF",
}

const fetchBranding = cache(async (): Promise<{ branding: Branding; suspended: boolean }> => {
  try {
    const host = await getRequestHost()
    const extraHeaders: Record<string, string> = {}
    if (host) extraHeaders["X-Forwarded-Host"] = host
    const data = await apiRequest<Branding>("/v1/branding", { headers: extraHeaders })
    return {
      branding: { ...DEFAULTS, ...data, colors: { ...DEFAULTS.colors, ...data.colors } },
      suspended: false,
    }
  } catch (error) {
    // /v1/branding is public, so a 403 can only mean the tenant is suspended.
    const suspended = typeof error === "object" && error !== null && (error as { status?: number }).status === 403
    return { branding: DEFAULTS, suspended }
  }
})

export const getBranding = async (): Promise<Branding> => (await fetchBranding()).branding

export const isTenantSuspended = async (): Promise<boolean> => (await fetchBranding()).suspended
