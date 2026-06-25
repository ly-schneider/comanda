import { getCSRFToken } from "@/lib/csrf"

export type AuthorizedFetch = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export type TenantInfo = { id: string; slug: string; name: string }
export type OnboardingStatus = {
  completedAt: string | null
  steps: { brand: boolean; payments: boolean; domain: boolean }
}

export async function getTenant(fetchAuth: AuthorizedFetch): Promise<TenantInfo> {
  const res = await fetchAuth("/api/v1/account/tenant")
  if (!res.ok) throw new Error(`Fehler ${res.status}`)
  return res.json() as Promise<TenantInfo>
}

export async function getOnboardingStatus(fetchAuth: AuthorizedFetch): Promise<OnboardingStatus | null> {
  const res = await fetchAuth("/api/v1/account/onboarding")
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
export async function getAccountLegal(fetchAuth: AuthorizedFetch): Promise<AccountLegal | null> {
  const res = await fetchAuth("/api/v1/account/settings")
  if (!res.ok) return null
  const data = (await res.json()) as Partial<AccountLegal>
  return {
    legalName: data.legalName ?? null,
    legalAddress: data.legalAddress ?? null,
    contactEmail: data.contactEmail ?? null,
  }
}

export async function completeOnboarding(fetchAuth: AuthorizedFetch): Promise<boolean> {
  const res = await fetchAuth("/api/v1/account/onboarding/complete", {
    method: "POST",
    headers: { "X-CSRF": getCSRFToken() || "" },
  })
  return res.ok || res.status === 204
}
