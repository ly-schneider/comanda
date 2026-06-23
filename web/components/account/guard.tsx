"use client"

import { useRouter } from "next/navigation"
import { useEffect } from "react"
import { authClient } from "@/lib/auth/client"
import { postAuthPath } from "@/lib/post-auth"

// AccountGuard gates the vendor account hub. A vendor is a signed-in user whose
// session has an active organization; that org is the tenant the backend rebinds
// to via ResolveVendorTenant. This is UI-only — the backend account routes are
// authoritative (membership is verified on the privileged pool). Non-vendors are
// routed back via postAuthPath (platform admins → /platform-admin, else → /).
export function AccountGuard({ children }: { children: React.ReactNode }) {
  const { data: session, isPending } = authClient.useSession()
  const router = useRouter()

  const activeOrganizationId = session?.session?.activeOrganizationId ?? null
  const isVendor = !!activeOrganizationId

  useEffect(() => {
    if (isPending) return
    if (!session?.user) {
      router.replace("/login?next=/account")
      return
    }
    if (!isVendor) {
      router.replace(
        postAuthPath(
          {
            isPlatformAdmin: (session.user as Record<string, unknown>).isPlatformAdmin === true,
            activeOrganizationId,
          },
          null
        )
      )
    }
  }, [isPending, session, isVendor, activeOrganizationId, router])

  if (isPending) {
    return <div className="text-muted-foreground p-8 text-center text-sm font-medium">Zugriff wird geprüft…</div>
  }

  if (!isVendor) return null

  return <>{children}</>
}
