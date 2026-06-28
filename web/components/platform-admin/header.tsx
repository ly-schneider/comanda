"use client"
import Link from "next/link"
import { UserMenu } from "@/components/user-menu"

export function PlatformAdminHeader() {
  return (
    <header className="border-border flex items-center justify-between border-b px-4 py-3">
      <Link href="/platform-admin" className="font-primary text-lg tracking-tight">
        Comanda <span className="text-muted-foreground">Platform</span>
      </Link>
      <UserMenu />
    </header>
  )
}
