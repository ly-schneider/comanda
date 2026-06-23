"use client"

import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/contexts/auth-context"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"

type TenantInfo = { id: string; slug: string; name: string }

export default function AccountPage() {
  const { signOut } = useAuth()
  const fetchAuth = useAuthorizedFetch()
  const [tenant, setTenant] = useState<TenantInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    ;(async () => {
      try {
        const res = await fetchAuth("/api/v1/account/tenant")
        if (!res.ok) throw new Error(`Error ${res.status}`)
        setTenant((await res.json()) as TenantInfo)
      } catch (e) {
        setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
      } finally {
        setLoading(false)
      }
    })()
  }, [fetchAuth])

  return (
    <div className="container mx-auto max-w-2xl px-4 py-12">
      <div className="flex items-center justify-between">
        <h1 className="font-primary text-2xl">
          {loading ? "Laden…" : error ? "Konto" : `Willkommen — ${tenant?.name ?? ""}`}
        </h1>
        <Button variant="outline" onClick={() => signOut()}>
          Abmelden
        </Button>
      </div>
      {error && (
        <div
          role="alert"
          className="bg-destructive/10 text-destructive mt-4 rounded-[var(--radius-control)] px-3 py-2 text-sm"
        >
          {error}
        </div>
      )}
    </div>
  )
}
