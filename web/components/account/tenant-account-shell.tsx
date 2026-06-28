"use client"

import { useRouter } from "next/navigation"
import { useEffect, useMemo, useState } from "react"
import { mergeBranding, TenantBrandingProvider } from "@/contexts/branding-context"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getManageableTenants, type ManageableTenant } from "@/lib/api/account"
import { buildThemeStyle } from "@/lib/branding-theme"
import type { Branding } from "@/lib/server/branding"
import { AccountTenantContext } from "./account-tenant-context"
import { AccountGuard } from "./guard"
import { AccountHeader } from "./header"

// TenantAccountShell scopes the account hub to the {tenant} slug in the URL. The
// slug is the single source of truth for which tenant every /api/v1/account/<slug>
// call targets, so switching is plain navigation — tab-safe, no shared cookie.
export function TenantAccountShell({ tenant, children }: { tenant: string; children: React.ReactNode }) {
  return (
    <AccountGuard>
      <TenantBoundary tenant={tenant}>{children}</TenantBoundary>
    </AccountGuard>
  )
}

function TenantBoundary({ tenant, children }: { tenant: string; children: React.ReactNode }) {
  const fetchAuth = useAuthorizedFetch()
  const router = useRouter()
  const [tenants, setTenants] = useState<ManageableTenant[] | null>(null)

  useEffect(() => {
    let active = true
    ;(async () => {
      try {
        const list = await getManageableTenants(fetchAuth)
        if (active) setTenants(list)
      } catch {
        if (active) setTenants([])
      }
    })()
    return () => {
      active = false
    }
  }, [fetchAuth])

  useEffect(() => {
    if (tenants === null) return
    if (tenants.length === 0) {
      router.replace("/")
      return
    }
    // The slug isn't one this user may manage — send them to their first tenant.
    if (!tenants.some((x) => x.slug === tenant)) {
      router.replace(`/t/${tenants[0]!.slug}`)
    }
  }, [tenants, tenant, router])

  const value = useMemo(() => {
    if (!tenants) return null
    const activeTenant = tenants.find((x) => x.slug === tenant)
    if (!activeTenant) return null
    return {
      tenants,
      activeSlug: tenant,
      activeTenant,
      switchTo: (slug: string) => router.push(`/t/${slug}`),
    }
  }, [tenants, tenant, router])

  if (!value) {
    return <div className="text-muted-foreground p-8 text-center text-sm font-medium">Wird geladen…</div>
  }

  return (
    <AccountTenantContext.Provider value={value}>
      <AccountBrandedShell key={tenant} tenant={tenant}>
        {children}
      </AccountBrandedShell>
    </AccountTenantContext.Provider>
  )
}

// AccountBrandedShell fetches the active tenant's branding and scopes it to this
// subtree via an inline style — the root :root vars stay neutral for the rest of
// the app on the platform host.
function AccountBrandedShell({ tenant, children }: { tenant: string; children: React.ReactNode }) {
  const fetchAuth = useAuthorizedFetch()
  const [branding, setBranding] = useState<Branding | null>(null)

  useEffect(() => {
    let active = true
    ;(async () => {
      try {
        const res = await fetchAuth(`/api/v1/account/${tenant}/branding`)
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
  }, [fetchAuth, tenant])

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
