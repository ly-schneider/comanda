"use client"

import { useEffect, useState } from "react"
import { mergeBranding, TenantBrandingProvider } from "@/contexts/branding-context"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { buildThemeStyle } from "@/lib/branding-theme"
import type { Branding } from "@/lib/server/branding"
import { AccountGuard } from "./guard"
import { AccountHeader } from "./header"

export function AccountShell({ children }: { children: React.ReactNode }) {
  return (
    <AccountGuard>
      <AccountBrandedShell>{children}</AccountBrandedShell>
    </AccountGuard>
  )
}

// AccountBrandedShell only mounts once the guard confirms a vendor, so the
// branding fetch always resolves to that vendor's tenant. The platform host
// renders neutral branding at SSR (no tenant Host), so we fetch the vendor's
// branding client-side and scope it to this subtree via an inline style — the
// root <head> :root vars stay neutral for the rest of the app.
function AccountBrandedShell({ children }: { children: React.ReactNode }) {
  const fetchAuth = useAuthorizedFetch()
  const [branding, setBranding] = useState<Branding | null>(null)

  useEffect(() => {
    let active = true
    ;(async () => {
      try {
        const res = await fetchAuth("/api/v1/account/branding")
        if (!res.ok) return
        const data = (await res.json()) as Partial<Branding>
        if (active) setBranding(mergeBranding(data))
      } catch {
        // keep neutral defaults on failure
      }
    })()
    return () => {
      active = false
    }
  }, [fetchAuth])

  const effective = branding ?? mergeBranding(null)

  return (
    <TenantBrandingProvider branding={effective}>
      <div style={buildThemeStyle(effective)} className="flex min-h-screen flex-col">
        <AccountHeader />
        <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8">{children}</main>
      </div>
    </TenantBrandingProvider>
  )
}
