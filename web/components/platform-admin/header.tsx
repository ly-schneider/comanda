"use client"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/contexts/auth-context"

export function PlatformAdminHeader() {
  const { signOut } = useAuth()
  return (
    <header className="border-border flex items-center justify-between border-b px-4 py-3">
      <Link href="/platform-admin" className="font-primary text-lg tracking-tight">
        Comanda <span className="text-muted-foreground">Platform</span>
      </Link>
      <Button variant="outline" size="sm" onClick={() => signOut()}>
        Abmelden
      </Button>
    </header>
  )
}
