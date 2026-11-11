import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { LegalPage, LegalRows, LegalText } from "@/components/marketing/legal-page"
import { PLATFORM_OPERATOR } from "@/lib/legal"
import { isPlatformRequest } from "@/lib/server/host"

export const metadata: Metadata = {
  title: "Impressum",
  description: "Wer hinter Comanda steht und wie du mich erreichst.",
}

export default async function ImpressumPage() {
  if (!(await isPlatformRequest())) notFound()

  const [street, city] = PLATFORM_OPERATOR.addressLines
  return (
    <LegalPage
      doc="Impressum"
      lead="Wer hinter Comanda steht und wie du mich erreichst."
      meta="Stand: 11. November 2026"
      sections={[
        {
          id: "betreiber",
          title: "Betreiber",
          content: (
            <>
              <LegalRows
                rows={[
                  ["Firma", PLATFORM_OPERATOR.legalName],
                  ["Inhaber", PLATFORM_OPERATOR.owner],
                  ["Adresse", street, `${city}, ${PLATFORM_OPERATOR.country}`],
                  [
                    "E-Mail",
                    <a key="mail" href={`mailto:${PLATFORM_OPERATOR.email}`}>
                      {PLATFORM_OPERATOR.email}
                    </a>,
                  ],
                  [
                    "Website",
                    <a key="web" href="https://www.leys.ch">
                      leys.ch
                    </a>,
                  ],
                ]}
              />
              <LegalText>
                Bestellst du an einem Stand, findest du das Impressum des jeweiligen Veranstalters in seinem
                Bestellshop.
              </LegalText>
            </>
          ),
        },
        {
          id: "inhalt",
          title: "Verantwortlich für den Inhalt",
          content: <LegalText>{PLATFORM_OPERATOR.owner}, Adresse wie oben.</LegalText>,
        },
        {
          id: "haftung",
          title: "Haftung für Inhalte",
          content: (
            <LegalText>
              Ich gebe mir Mühe, dass alles auf dieser Website stimmt. Eine Garantie für Richtigkeit, Vollständigkeit
              und Aktualität kann ich trotzdem nicht übernehmen. Haftungsansprüche wegen Schäden aus der Nutzung dieser
              Informationen sind ausgeschlossen, soweit das Gesetz es zulässt.
            </LegalText>
          ),
        },
        {
          id: "links",
          title: "Links auf andere Websites",
          content: (
            <LegalText>
              Für die Inhalte externer Websites, auf die ich verlinke, sind deren Betreiber verantwortlich. Beim Setzen
              der Links waren keine Rechtsverstösse erkennbar.
            </LegalText>
          ),
        },
        {
          id: "urheberrecht",
          title: "Urheberrecht",
          content: (
            <LegalText>
              Texte, Bilder und Gestaltung dieser Website gehören {PLATFORM_OPERATOR.legalName}. Für den Quellcode von
              Comanda gilt die Business Source License 1.1. Was das heisst, steht auf der Seite{" "}
              <Link href="/lizenz" className="underline underline-offset-4">
                Lizenz
              </Link>
              .
            </LegalText>
          ),
        },
      ]}
    />
  )
}
