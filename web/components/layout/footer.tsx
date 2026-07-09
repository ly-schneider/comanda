"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { VersionLabel } from "@/components/layout/version-label"
import { useCart } from "@/contexts/cart-context"

export default function Footer({ version }: { version?: string }) {
  const pathname = usePathname()
  const { cart } = useCart()
  const totalItems = cart.items.reduce((sum, it) => sum + it.quantity, 0)

  if (pathname?.startsWith("/checkout")) return null

  return (
    <footer
      id="app-footer"
      className={`text-muted-foreground w-full py-4 text-sm ${
        pathname === "/" && totalItems > 0 ? "mb-20" : pathname === "/orders" ? "mb-18" : "mb-4"
      }`}
    >
      <div className="container mx-auto px-4">
        <nav className="flex flex-wrap items-center justify-center gap-x-2 gap-y-1">
          <VersionLabel version={version} />
          <span className="text-gray-300">&middot;</span>
          <Link href="/datenschutz" className="hover:underline">
            Datenschutz
          </Link>
          <span className="text-gray-300">&middot;</span>
          <Link href="/nutzungsbedingungen" className="hover:underline">
            Nutzungsbedingungen
          </Link>
        </nav>
      </div>
    </footer>
  )
}
