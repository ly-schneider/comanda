import { ArrowUpRight, Check, Mail, X } from "lucide-react"
import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { LegalList, LegalPage, LegalText } from "@/components/marketing/legal-page"
import { CONTACT_EMAIL, LINKS } from "@/lib/marketing"
import { isPlatformRequest } from "@/lib/server/host"
import { cn } from "@/lib/utils"

export const metadata: Metadata = {
  title: "Lizenz",
  description: "Der Code von Comanda ist öffentlich. Was du damit machen darfst, in einfachen Worten.",
}

const SUMMARY = [
  {
    icon: Check,
    tone: "bg-success/10 text-success",
    title: "Erlaubt",
    text: "Selber betreiben, wenn ihr ein Verein, eine Non-Profit oder eine kleine Organisation seid.",
  },
  {
    icon: X,
    tone: "bg-destructive/10 text-destructive",
    title: "Nicht erlaubt",
    text: "Comanda als gehosteten Dienst anbieten oder in ein Produkt einbauen, das du verkaufst.",
  },
  {
    icon: Mail,
    tone: "bg-muted-foreground/10 text-foreground",
    title: "Lizenz nötig",
    text: "Grössere Organisationen brauchen für den Live-Betrieb eine kommerzielle Lizenz.",
  },
]

export default async function LizenzPage() {
  if (!(await isPlatformRequest())) notFound()

  return (
    <LegalPage
      doc="Lizenz"
      lead="Der Code von Comanda ist öffentlich. Was du damit machen darfst, steht hier in einfachen Worten."
      meta="Business Source License 1.1 · Stand: 11. November 2026"
      sections={[
        {
          id: "kurz-gesagt",
          title: "Kurz gesagt",
          content: (
            <div className="grid gap-3 sm:grid-cols-3">
              {SUMMARY.map(({ icon: Icon, tone, title, text }) => (
                <div key={title} className="bg-secondary flex flex-col gap-3 rounded-[var(--radius-surface)] p-[22px]">
                  <span className={cn("rounded-pill w-fit p-2", tone)}>
                    <Icon className="size-4" />
                  </span>
                  <p className="text-[15px] font-semibold">{title}</p>
                  <p className="text-muted-foreground text-[15px] leading-6">{text}</p>
                </div>
              ))}
            </div>
          ),
        },
        {
          id: "gratis",
          title: "Wer Comanda gratis nutzen darf",
          content: (
            <>
              <LegalText>
                Du darfst Comanda selber betreiben, auch live an deinem Anlass, wenn eines davon auf dich zutrifft:
              </LegalText>
              <LegalList
                items={[
                  "Du bist eine Privatperson, ein Verein, eine Non-Profit-Organisation oder eine Bildungseinrichtung.",
                  "Deine Organisation hat zusammen mit verbundenen Unternehmen weniger als 50 Mitarbeitende und weniger als 5 Millionen US-Dollar Jahresumsatz.",
                ]}
              />
              <LegalText>
                Selber betreiben heisst: für deine eigene Organisation, deine Mitglieder und eure Gäste.
              </LegalText>
            </>
          ),
        },
        {
          id: "nicht-erlaubt",
          title: "Was nicht erlaubt ist",
          content: (
            <LegalText>
              Comanda oder eine veränderte Version davon anderen als gehosteten oder verwalteten Dienst anbieten. Und es
              in ein Produkt oder einen Dienst einbauen, den du an Dritte verkaufst oder lizenzierst.
            </LegalText>
          ),
        },
        {
          id: "gross",
          title: "Für grössere Organisationen",
          content: (
            <LegalText>
              Liegt ihr über diesen Grenzen, braucht ihr für den Live-Betrieb eine kommerzielle Lizenz. Schreib mir an{" "}
              <a href={`mailto:${CONTACT_EMAIL}?subject=Comanda-Lizenz`} className="underline underline-offset-4">
                {CONTACT_EMAIL}
              </a>
              , dann finden wir eine Lösung.
            </LegalText>
          ),
        },
        {
          id: "agpl",
          title: "Nach vier Jahren gilt die AGPL",
          content: (
            <LegalText>
              Jede Version von Comanda steht vier Jahre nach ihrer Veröffentlichung automatisch unter der GNU Affero
              General Public License v3.0 oder neuer. Für diese Version fallen die Einschränkungen oben dann weg.
            </LegalText>
          ),
        },
        {
          id: "ausprobieren",
          title: "Ausprobieren ist immer erlaubt",
          content: (
            <LegalText>
              Lesen, verändern und zum Entwickeln, Testen oder Ausprobieren laufen lassen darfst du Comanda in jedem
              Fall.
            </LegalText>
          ),
        },
        {
          id: "volltext",
          title: "Der vollständige Text",
          content: (
            <>
              <LegalText>
                Rechtlich verbindlich ist nur der englische Lizenztext der Business Source License 1.1 im Repository.
                Diese Seite fasst ihn zusammen.
              </LegalText>
              <a
                href={LINKS.license}
                className="inline-flex items-center gap-1.5 text-[15px] font-medium hover:underline"
              >
                LICENSE auf GitHub lesen
                <ArrowUpRight className="size-4" />
              </a>
            </>
          ),
        },
      ]}
    />
  )
}
