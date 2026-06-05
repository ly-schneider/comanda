"use client"

import { createContext, useContext } from "react"
import type { Branding } from "@/lib/server/branding"

const DEFAULTS: Branding = {
  displayName: "Comanda",
  logoUrl: null,
  faviconUrl: null,
  colors: {
    primary: "#f87778",
    primaryForeground: "#fdfdfd",
    background: "#e9e7e6",
    foreground: "#000000",
  },
  contact: { email: null, phone: null },
  legal: { name: null, address: null, text: null },
  currency: "CHF",
}

const BrandingContext = createContext<Branding>(DEFAULTS)

export function TenantBrandingProvider({ branding, children }: { branding: Branding; children: React.ReactNode }) {
  return <BrandingContext.Provider value={branding}>{children}</BrandingContext.Provider>
}

export function useBranding(): Branding {
  return useContext(BrandingContext)
}
