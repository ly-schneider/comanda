import Link from "next/link"
import { LEGAL_DOCS } from "@/lib/marketing"
import { cn } from "@/lib/utils"
import { LegalToc } from "./legal-toc"
import { SiteFooter } from "./site-footer"
import { SiteNav } from "./site-nav"

export type LegalSection = { id: string; title: string; content: React.ReactNode }

export function LegalPage({
  doc,
  lead,
  meta,
  sections,
}: {
  doc: (typeof LEGAL_DOCS)[number]["label"]
  lead: string
  meta: string
  sections: LegalSection[]
}) {
  return (
    <>
      <SiteNav />
      <main>
        <header className="mx-auto w-full max-w-[1200px] px-4 pt-16 pb-12 sm:px-6 sm:pt-24 sm:pb-16 xl:px-0">
          <p className="flex gap-2 text-[13px] font-medium">
            <span className="text-muted-foreground">Rechtliches</span>
            <span className="text-border">/</span>
            <span>{doc}</span>
          </p>
          <h1 className="mt-5 text-5xl leading-[1.05] font-semibold tracking-[-0.03em] sm:text-[64px] sm:leading-[68px]">
            {doc}
          </h1>
          <p className="text-muted-foreground mt-5 max-w-[620px] text-[17px] leading-7 sm:text-[19px] sm:leading-[30px]">
            {lead}
          </p>
          <p className="text-muted-foreground mt-5 text-[13px]">{meta}</p>
        </header>
        <div className="border-border border-t">
          <div className="mx-auto flex w-full max-w-[1200px] gap-[120px] px-4 pt-12 pb-24 sm:px-6 sm:pb-32 xl:px-0">
            <aside className="sticky top-24 hidden h-fit w-[260px] shrink-0 lg:block">
              <p className="text-muted-foreground text-[13px] font-medium">Rechtliches</p>
              <ul className="mt-2 flex flex-col gap-0.5">
                {LEGAL_DOCS.map((d) => (
                  <li key={d.href}>
                    <Link
                      href={d.href}
                      aria-current={d.label === doc ? "page" : undefined}
                      className={cn(
                        "block rounded-[var(--radius-control)] px-3 py-2 text-[15px] font-medium transition-colors",
                        d.label === doc ? "bg-secondary" : "text-muted-foreground hover:text-foreground"
                      )}
                    >
                      {d.label}
                    </Link>
                  </li>
                ))}
              </ul>
              <p className="text-muted-foreground mt-8 mb-2 text-[13px] font-medium">Auf dieser Seite</p>
              <LegalToc sections={sections.map(({ id, title }) => ({ id, title }))} />
            </aside>
            <div className="flex w-full max-w-[720px] flex-col gap-14">
              {sections.map((s, i) => (
                <section key={s.id} id={s.id} className="flex scroll-mt-24 flex-col gap-[18px]">
                  <h2 className="text-2xl leading-[30px] font-semibold tracking-[-0.015em]">
                    {i + 1}. {s.title}
                  </h2>
                  {s.content}
                </section>
              ))}
            </div>
          </div>
        </div>
      </main>
      <SiteFooter />
    </>
  )
}

export function LegalText({ children }: { children: React.ReactNode }) {
  return <p className="text-[17px] leading-[29px] tracking-[-0.003em]">{children}</p>
}

export function LegalList({ items }: { items: React.ReactNode[] }) {
  return (
    <ul className="flex flex-col gap-2.5">
      {items.map((item, i) => (
        <li key={i} className="flex gap-3.5 text-[17px] leading-[29px]">
          <span className="bg-muted-foreground mt-3 size-[5px] shrink-0 rounded-full" />
          <span>{item}</span>
        </li>
      ))}
    </ul>
  )
}

export function LegalRows({
  rows,
  labelWidth = "sm:w-[180px]",
}: {
  rows: [string, React.ReactNode, React.ReactNode?][]
  labelWidth?: string
}) {
  return (
    <dl className="border-border border-b">
      {rows.map(([label, value, sub]) => (
        <div
          key={label}
          className="border-border flex flex-col gap-1 border-t py-3.5 text-[15px] leading-6 sm:flex-row sm:gap-6"
        >
          <dt className={cn("text-muted-foreground shrink-0", labelWidth)}>{label}</dt>
          <dd>
            <span className="block font-medium">{value}</span>
            {sub ? <span className="text-muted-foreground block">{sub}</span> : null}
          </dd>
        </div>
      ))}
    </dl>
  )
}
