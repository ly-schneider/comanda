"use client"
import { useRouter } from "next/navigation"
import { useEffect } from "react"
import { useMemberTier } from "@/hooks/use-member-tier"

export function AdminGuard({ children }: { children: React.ReactNode }) {
  const { isMember, isLoading } = useMemberTier()
  const router = useRouter()

  useEffect(() => {
    if (!isLoading && !isMember) {
      router.replace("/")
    }
  }, [isLoading, isMember, router])

  if (isLoading) {
    return (
      <div className="items-center justify-center text-center text-sm font-semibold text-gray-600">
        Zugriff wird geprüft...
      </div>
    )
  }

  if (!isMember) return null

  return <>{children}</>
}
