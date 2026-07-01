"use client"

import Image from "next/image"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { UserMenu } from "@/components/user-menu"
import { useBranding } from "@/contexts/branding-context"
import { useAccountTenant } from "./account-tenant-context"

// AccountHeader is the active-tenant-branded chrome; the switcher shows when the
// user manages more than one tenant.
export function AccountHeader() {
  const branding = useBranding()
  const pathname = usePathname()
  const { tenants, activeSlug, switchTo } = useAccountTenant()

  const base = `/t/${activeSlug}`
  const nav = [
    { href: base, label: "Übersicht", exact: true },
    { href: `${base}/branding`, label: "Marke", exact: false },
    { href: `${base}/payrexx`, label: "Payrexx", exact: false },
    { href: `${base}/sumup`, label: "SumUp", exact: false },
    { href: `${base}/domains`, label: "Domains", exact: false },
    { href: `${base}/invites`, label: "Einladungen", exact: false },
  ]

  return (
    <header className="border-border flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
      <div className="flex flex-wrap items-center gap-6">
        <Link href={base} className="flex items-center gap-2">
          {branding.logoUrl && (
            <Image src={branding.logoUrl} alt={branding.displayName} width={28} height={28} className="rounded-lg" />
          )}
          <span className="font-primary text-lg tracking-tight">{branding.displayName}</span>
        </Link>
        {tenants.length > 1 && (
          <Select value={activeSlug} onValueChange={switchTo}>
            <SelectTrigger className="h-8 w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {tenants.map((t) => (
                <SelectItem key={t.slug} value={t.slug}>
                  {t.displayName || t.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        <nav className="flex items-center gap-1">
          {nav.map((item) => {
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
      <UserMenu />
    </header>
  )
}
