"use client"

import Image from "next/image"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/contexts/auth-context"
import { useBranding } from "@/contexts/branding-context"

const NAV = [
  { href: "/account", label: "Übersicht", exact: true },
  { href: "/account/branding", label: "Marke", exact: false },
  { href: "/account/domains", label: "Domains", exact: false },
]

// AccountHeader is the vendor-branded chrome for the account hub. Unlike the
// platform-admin header (hardcoded "Comanda"), it shows the logged-in vendor's
// logo/name from the branding resolved by /api/v1/account/branding.
export function AccountHeader() {
  const { signOut } = useAuth()
  const branding = useBranding()
  const pathname = usePathname()

  return (
    <header className="border-border flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
      <div className="flex flex-wrap items-center gap-6">
        <Link href="/account" className="flex items-center gap-2">
          {branding.logoUrl && (
            <Image src={branding.logoUrl} alt={branding.displayName} width={28} height={28} className="rounded" />
          )}
          <span className="font-primary text-lg tracking-tight">{branding.displayName}</span>
        </Link>
        <nav className="flex items-center gap-1">
          {NAV.map((item) => {
            const active = item.exact ? pathname === item.href : pathname.startsWith(item.href)
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`rounded-[var(--radius-control)] px-3 py-1.5 text-sm transition-colors ${
                  active ? "bg-muted font-medium" : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {item.label}
              </Link>
            )
          })}
        </nav>
      </div>
      <Button variant="outline" size="sm" onClick={() => signOut()}>
        Abmelden
      </Button>
    </header>
  )
}
