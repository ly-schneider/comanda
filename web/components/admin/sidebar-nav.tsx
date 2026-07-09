"use client"
import {
  Circle,
  ClipboardList,
  CreditCard,
  Grid2x2,
  Home,
  MailPlus,
  MonitorCheck,
  Package,
  ReceiptText,
  Settings,
  Smartphone,
  Tag,
  Ticket,
  Undo2,
  Users,
} from "lucide-react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { VersionLabel } from "@/components/layout/version-label"
import { useMemberTier } from "@/hooks/use-member-tier"
import type { MemberTier } from "@/lib/auth/rbac"

type NavItem = {
  href: string
  label: string
  icon: React.ReactNode
  badge?: number
  requiredTier: MemberTier
}

export function AdminSidebar({ badges, version }: { badges?: Partial<Record<string, number>>; version?: string }) {
  const pathname = usePathname()
  const { tierAllows } = useMemberTier()

  const allItems: NavItem[] = [
    { href: "/admin", label: "Home", icon: <Home className="size-5" />, requiredTier: "catalog" },
    { href: "/admin/users", label: "Benutzer", icon: <Users className="size-5" />, requiredTier: "app_admin" },
    {
      href: "/admin/invites",
      label: "Einladungen",
      icon: <MailPlus className="size-5" />,
      badge: badges?.invites,
      requiredTier: "app_admin",
    },
    { href: "/admin/products", label: "Produkte", icon: <Tag className="size-5" />, requiredTier: "catalog" },
    { href: "/admin/bundles", label: "Bundles", icon: <Package className="size-5" />, requiredTier: "catalog" },
    { href: "/admin/categories", label: "Kategorien", icon: <Grid2x2 className="size-5" />, requiredTier: "catalog" },
    {
      href: "/admin/orders",
      label: "Bestellungen",
      icon: <ReceiptText className="size-5" />,
      badge: badges?.orders,
      requiredTier: "staff",
    },
    {
      href: "/admin/refunds",
      label: "Rückerstattungen",
      icon: <Undo2 className="size-5" />,
      requiredTier: "staff",
    },
    { href: "/admin/inventory", label: "Inventar", icon: <ClipboardList className="size-5" />, requiredTier: "staff" },
    { href: "/admin/devices", label: "Geräte", icon: <Smartphone className="size-5" />, requiredTier: "staff" },
    {
      href: "/admin/stations",
      label: "Stationen",
      icon: <MonitorCheck className="size-5" />,
      requiredTier: "staff",
    },
    { href: "/admin/pos", label: "POS", icon: <CreditCard className="size-5" />, requiredTier: "staff" },
    { href: "/admin/jetons", label: "Jetons", icon: <Circle className="size-5" />, requiredTier: "staff" },
    {
      href: "/admin/staff-perks",
      label: "Mitarbeiter-Gutscheine",
      icon: <Ticket className="size-5" />,
      requiredTier: "staff",
    },
    {
      href: "/admin/settings",
      label: "Einstellungen",
      icon: <Settings className="size-5" />,
      requiredTier: "app_admin",
    },
  ]

  const items = allItems.filter((it) => tierAllows(it.requiredTier))

  return (
    <nav aria-label="Hauptnavigation" className="w-full">
      {/* Mobile: horizontal badges */}
      <div className="flex flex-wrap gap-2 px-6 md:hidden md:px-8 lg:px-10">
        {items.map((it) => {
          const active = pathname === it.href
          return (
            <Link
              key={it.href}
              href={it.href}
              className={`inline-flex h-9 items-center gap-2 rounded-full border px-3 ${
                active ? "bg-muted font-semibold" : "bg-card"
              }`}
              aria-current={active ? "page" : undefined}
            >
              {it.icon}
              <span className="text-sm">{it.label}</span>
              {typeof it.badge === "number" && it.badge > 0 && (
                <span className="bg-destructive rounded-full px-1.5 py-0.5 text-xs text-white">{it.badge}</span>
              )}
            </Link>
          )
        })}
        <VersionLabel className="mt-1 w-full text-center" version={version} />
      </div>
      {/* Desktop: vertical rounded panel with margin spacing from left */}
      <div className="hidden md:block">
        <div className="bg-card ml-6 max-w-md rounded-[11px] p-1 shadow-sm md:ml-8 lg:ml-10">
          {items.map((it) => {
            const active = pathname === it.href
            return (
              <Link
                key={it.href}
                href={it.href}
                className={`hover:bg-accent-hover flex h-10 items-center justify-between gap-2 rounded-lg px-3 ${
                  active ? "bg-muted font-medium" : ""
                }`}
                aria-current={active ? "page" : undefined}
              >
                <span className="flex items-center gap-2.5 text-sm">
                  {it.icon}
                  {it.label}
                </span>
                {typeof it.badge === "number" && it.badge > 0 && (
                  <span className="bg-destructive rounded-full px-1.5 py-0.5 text-xs text-white">{it.badge}</span>
                )}
              </Link>
            )
          })}
        </div>
        <VersionLabel className="mt-2 ml-6 block max-w-md text-center md:ml-8 lg:ml-10" version={version} />
      </div>
    </nav>
  )
}

export function AdminShell({
  children,
  badges,
  version,
}: {
  children: React.ReactNode
  badges?: Partial<Record<string, number>>
  version?: string
}) {
  return (
    <div className="min-h-dvh w-full overflow-x-clip">
      <div className="mx-auto w-full">
        <div className="pt-3 md:hidden">
          <AdminSidebar badges={badges} version={version} />
        </div>
        <div className="hidden md:grid md:grid-cols-[300px_1fr] md:gap-6">
          <div>
            <AdminSidebar badges={badges} version={version} />
          </div>
          <div className="min-w-0 pr-6 md:pr-8 lg:pr-10">{children}</div>
        </div>
        <div className="min-w-0 px-6 pt-4 pb-10 md:hidden md:px-8 lg:px-10">{children}</div>
      </div>
    </div>
  )
}
