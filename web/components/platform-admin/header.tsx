"use client"
import Link from "next/link"
import { UserMenu } from "@/components/user-menu"

export function PlatformAdminHeader() {
  return (
    <header className="border-border flex items-center justify-between border-b px-4 py-3">
      <div className="flex items-center gap-6">
        <Link href="/platform-admin" className="font-primary text-lg tracking-tight">
          Comanda <span className="text-muted-foreground">Platform</span>
        </Link>
        <nav className="flex items-center gap-4 text-sm">
          <Link href="/platform-admin" className="text-muted-foreground hover:text-foreground">
            Tenants
          </Link>
          <Link href="/platform-admin/qa" className="text-muted-foreground hover:text-foreground">
            QA-Checkliste
          </Link>
        </nav>
      </div>
      <UserMenu />
    </header>
  )
}
