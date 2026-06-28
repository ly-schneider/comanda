"use client"

import { useRouter } from "next/navigation"
import { useEffect } from "react"
import { authClient } from "@/lib/auth/client"

// AccountGuard gates the account hub on a signed-in user. Which tenants the user
// may actually manage (and whether they have any) is resolved by the tenant
// boundary, which lists them and sends a user with none away. The backend
// account routes remain authoritative (membership / platform-admin verified on
// the privileged pool).
export function AccountGuard({ children }: { children: React.ReactNode }) {
  const { data: session, isPending } = authClient.useSession()
  const router = useRouter()

  useEffect(() => {
    if (isPending) return
    if (!session?.user) {
      router.replace("/login?next=/account")
    }
  }, [isPending, session, router])

  if (isPending) {
    return <div className="text-muted-foreground p-8 text-center text-sm font-medium">Zugriff wird geprüft…</div>
  }

  if (!session?.user) return null

  return <>{children}</>
}
