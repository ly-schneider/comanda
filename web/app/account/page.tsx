"use client"

import { useRouter } from "next/navigation"
import { useEffect } from "react"
import { AccountGuard } from "@/components/account/guard"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getManageableTenants } from "@/lib/api/account"

// Stable post-auth entry point; forwards to the slug-scoped hub at /t/<tenant>.
export default function AccountIndex() {
  return (
    <AccountGuard>
      <Redirector />
    </AccountGuard>
  )
}

function Redirector() {
  const fetchAuth = useAuthorizedFetch()
  const router = useRouter()

  useEffect(() => {
    let active = true
    ;(async () => {
      try {
        const list = await getManageableTenants(fetchAuth)
        if (!active) return
        router.replace(list.length > 0 ? `/t/${list[0]!.slug}` : "/")
      } catch {
        if (active) router.replace("/")
      }
    })()
    return () => {
      active = false
    }
  }, [fetchAuth, router])

  return <div className="text-muted-foreground p-8 text-center text-sm font-medium">Wird geladen…</div>
}
