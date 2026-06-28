"use client"

import { useRouter } from "next/navigation"
import { useEffect } from "react"
import { authClient } from "@/lib/auth/client"

// AccountGuard gates the hub on a signed-in user; which tenants they may manage
// is resolved by the tenant boundary. The backend routes stay authoritative.
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
