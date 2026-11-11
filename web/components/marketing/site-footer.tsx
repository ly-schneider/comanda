import Image from "next/image"
import Link from "next/link"
import { CONTACT_EMAIL, LEGAL_DOCS, LINKS, NAV_LINKS } from "@/lib/marketing"

const COLUMNS = [
  { title: "Comanda", links: NAV_LINKS.slice(0, 3) },
  {
    title: "Kontakt",
    links: [
      { label: CONTACT_EMAIL, href: `mailto:${CONTACT_EMAIL}` },
      { label: "leys.ch", href: LINKS.leys },
      { label: "Blog", href: LINKS.blog },
    ],
  },
  { title: "Rechtliches", links: LEGAL_DOCS },
]

export function SiteFooter() {
  return (
    <footer className="border-border border-t">
      <div className="mx-auto max-w-[1200px] px-4 pt-16 pb-12 sm:px-6 xl:px-0">
        <div className="flex flex-col gap-12 md:flex-row md:justify-between">
          <div className="flex flex-col gap-4">
            <Image src="/assets/images/comanda-logo.png" alt="Comanda" width={28} height={28} />
            <p className="text-muted-foreground text-[15px] leading-6">
              Kasse und Handy-Bestellung
              <br />
              für Stände an Anlässen.
            </p>
          </div>
          <div className="grid grid-cols-2 gap-x-12 gap-y-10 sm:grid-cols-3 sm:gap-x-24">
            {COLUMNS.map((column) => (
              <div key={column.title} className="flex flex-col gap-3">
                <p className="text-[13px] font-semibold">{column.title}</p>
                <ul className="flex flex-col gap-3">
                  {column.links.map((link) => (
                    <li key={link.href}>
                      <Link
                        href={link.href}
                        className="text-muted-foreground hover:text-foreground text-[13px] transition-colors"
                      >
                        {link.label}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>
        <div className="border-border text-muted-foreground mt-16 flex flex-col gap-2 border-t pt-6 text-[13px] sm:flex-row sm:justify-between">
          <p>© {new Date().getFullYear()} Leys Services</p>
          <p>Mühlethurnen, Schweiz</p>
        </div>
      </div>
    </footer>
  )
}
