"use client"
import { useRouter } from "next/navigation"
import { useEffect } from "react"
import { useAuth } from "@/contexts/auth-context"

// PlatformAdminGuard gates the cross-tenant control plane. Platform admin is an
// orthogonal flag (user.is_platform_admin), not a tenant role — the backend
// RequirePlatformAdmin gate is authoritative; this is UI-only.
export function PlatformAdminGuard({ children }: { children: React.ReactNode }) {
  const { user, isLoading } = useAuth()
  const router = useRouter()

  const hasAccess = user?.isPlatformAdmin === true

  useEffect(() => {
    if (!isLoading && !hasAccess) {
      router.replace("/")
    }
  }, [isLoading, hasAccess, router])

  if (isLoading) {
    return <div className="text-muted-foreground p-8 text-center text-sm font-medium">Zugriff wird geprüft…</div>
  }

  if (!hasAccess) return null

  return <>{children}</>
}
