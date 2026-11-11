import { ArrowRight } from "lucide-react"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { CONTACT_MAILTO, NAV_LINKS } from "@/lib/marketing"
import { SiteFooter } from "./site-footer"
import { SiteNav } from "./site-nav"

export function MarketingNotFound() {
  return (
    <>
      <SiteNav />
      <main className="flex flex-col items-center px-4 pt-32 pb-40 text-center sm:pt-40 sm:pb-48">
        <span className="bg-secondary text-muted-foreground rounded-pill px-3 py-1 text-[13px] font-medium">404</span>
        <h1 className="mt-6 text-[44px] leading-[1.05] font-semibold tracking-[-0.035em] sm:text-6xl lg:text-[76px] lg:leading-[78px]">
          Hier gibt es
          <br />
          nichts zu bestellen.
        </h1>
        <p className="text-muted-foreground mt-6 max-w-[520px] text-[17px] leading-7 sm:text-[19px] sm:leading-[30px]">
          Die Seite, die du suchst, gibt es nicht oder sie ist umgezogen.
        </p>
        <div className="mt-8 flex flex-wrap justify-center gap-3">
          <Button asChild variant="primary" size="lg">
            <Link href="/">
              Zur Startseite
              <ArrowRight />
            </Link>
          </Button>
          <Button asChild variant="secondary" size="lg" className="rounded-pill">
            <a href={CONTACT_MAILTO}>Schreib mir</a>
          </Button>
        </div>
        <ul className="mt-12 flex flex-wrap justify-center gap-6">
          {[...NAV_LINKS.slice(0, 3), { label: "Datenschutz", href: "/datenschutz" }].map((link) => (
            <li key={link.href}>
              <Link href={link.href} className="text-muted-foreground hover:text-foreground text-[13px] font-medium">
                {link.label}
              </Link>
            </li>
          ))}
        </ul>
      </main>
      <SiteFooter />
    </>
  )
}
