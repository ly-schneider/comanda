"use client"

import { createContext, useContext } from "react"
import type { ManageableTenant } from "@/lib/api/account"

type AccountTenantValue = {
  tenants: ManageableTenant[]
  activeSlug: string
  activeTenant: ManageableTenant
  switchTo: (slug: string) => void
}

export const AccountTenantContext = createContext<AccountTenantValue | null>(null)

export function useAccountTenant(): AccountTenantValue {
  const ctx = useContext(AccountTenantContext)
  if (!ctx) throw new Error("useAccountTenant must be used within the account hub")
  return ctx
}
