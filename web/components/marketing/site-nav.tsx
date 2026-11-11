import Image from "next/image"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { CONTACT_MAILTO, NAV_LINKS } from "@/lib/marketing"

export function SiteNav() {
  return (
    <header className="bg-background/90 sticky top-0 z-40 backdrop-blur">
      <nav className="mx-auto flex h-16 max-w-[1200px] items-center justify-between px-4 sm:px-6 xl:px-0">
        <Link href="/" className="flex items-center gap-2.5">
          <Image src="/assets/images/comanda-logo.png" alt="" width={22} height={22} priority />
          <span className="text-[15px] font-semibold tracking-tight">Comanda</span>
        </Link>
        <ul className="hidden items-center gap-8 md:flex">
          {NAV_LINKS.map((link) => (
            <li key={link.href}>
              <Link
                href={link.href}
                className="text-muted-foreground hover:text-foreground text-[13px] font-medium transition-colors"
              >
                {link.label}
              </Link>
            </li>
          ))}
        </ul>
        <div className="flex items-center gap-5">
          <Link href="/login" className="hidden text-[13px] font-medium sm:block">
            Anmelden
          </Link>
          <Button asChild variant="primary" size="sm">
            <a href={CONTACT_MAILTO}>Schreib mir</a>
          </Button>
        </div>
      </nav>
    </header>
  )
}
