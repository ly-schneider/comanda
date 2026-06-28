import { getCSRFToken } from "@/lib/csrf"

export type AuthorizedFetch = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export type ManageableTenant = {
  id: string
  slug: string
  name: string
  displayName: string
  logoUrl?: string
  role?: string
}

// Lists the user's manageable tenants (owner/admin memberships, or all for a platform admin).
export async function getManageableTenants(fetchAuth: AuthorizedFetch): Promise<ManageableTenant[]> {
  const res = await fetchAuth("/api/v1/account/tenants")
  if (!res.ok) throw new Error(`Fehler ${res.status}`)
  const data = (await res.json()) as { items: ManageableTenant[] }
  return data.items ?? []
}
export type OnboardingStatus = {
  completedAt: string | null
  steps: { brand: boolean; payments: boolean; domain: boolean }
}

export async function getOnboardingStatus(
  fetchAuth: AuthorizedFetch,
  tenant: string
): Promise<OnboardingStatus | null> {
  const res = await fetchAuth(`/api/v1/account/${tenant}/onboarding`)
  if (!res.ok) return null
  return res.json() as Promise<OnboardingStatus>
}

export type AccountLegal = {
  legalName: string | null
  legalAddress: string | null
  contactEmail: string | null
}

// getAccountLegal reads the branding settings so the onboarding checklist can
// show which imprint fields the "Marke & Rechtliches" step is still missing.
export async function getAccountLegal(fetchAuth: AuthorizedFetch, tenant: string): Promise<AccountLegal | null> {
  const res = await fetchAuth(`/api/v1/account/${tenant}/settings`)
  if (!res.ok) return null
  const data = (await res.json()) as Partial<AccountLegal>
  return {
    legalName: data.legalName ?? null,
    legalAddress: data.legalAddress ?? null,
    contactEmail: data.contactEmail ?? null,
  }
}

export async function completeOnboarding(fetchAuth: AuthorizedFetch, tenant: string): Promise<boolean> {
  const res = await fetchAuth(`/api/v1/account/${tenant}/onboarding/complete`, {
    method: "POST",
    headers: { "X-CSRF": getCSRFToken() || "" },
  })
  return res.ok || res.status === 204
}
