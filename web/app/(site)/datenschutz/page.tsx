import { Metadata } from "next"
import { OperatorBlock } from "@/components/legal/operator-block"
import { PlatformPrivacy } from "@/components/marketing/platform-privacy"
import { FOREIGN_TRANSFERS, SUBPROCESSORS } from "@/lib/legal"
import { getBranding } from "@/lib/server/branding"
import { isPlatformRequest } from "@/lib/server/host"

export async function generateMetadata(): Promise<Metadata> {
  if (await isPlatformRequest()) {
    return {
      title: "Datenschutz",
      description: "Welche Daten auf comanda.leys.ch anfallen und was damit passiert, nach dem Schweizer nDSG.",
    }
  }
  const branding = await getBranding()
  return {
    title: "Datenschutzerklärung",
    description: `Datenschutzerklärung der ${branding.displayName} Plattform gemäss dem Schweizer Datenschutzgesetz (nDSG).`,
  }
}

export default async function PrivacyPolicyPage() {
  if (await isPlatformRequest()) return <PlatformPrivacy />

  const branding = await getBranding()
  const entityName = branding.legal.name || branding.displayName
  const address = branding.legal.address
  const email = branding.contact.email

  return (
    <div className="container mx-auto max-w-2xl px-4 py-8">
      <h1 className="mb-2 text-2xl font-semibold">Datenschutzerklärung</h1>
      <p className="text-muted-foreground mb-8 text-sm">Letzte Aktualisierung: 13. Juli 2026</p>

      <div className="space-y-8 text-sm leading-relaxed">
        <section>
          <p>
            Diese Datenschutzerklärung informiert Sie gemäss dem Schweizer Bundesgesetz über den Datenschutz (nDSG)
            darüber, wie wir Ihre Personendaten im Zusammenhang mit der Nutzung der {branding.displayName} System
            (nachfolgend «Plattform») bearbeiten.
          </p>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">1. Verantwortliche Stelle und Plattformbetreiber</h2>
          <p>Verantwortliche Stelle für die Bearbeitung Ihrer Personendaten ist der Anbieter, bei dem Sie bestellen:</p>
          <p className="mt-2">
            {entityName}
            {address ? (
              <>
                <br />
                {address}
              </>
            ) : null}
            <br />
            Schweiz
          </p>
          {email ? <p className="mt-2">E-Mail: {email}</p> : null}

          <p className="mt-4">Die technische Plattform wird im Auftrag des Anbieters betrieben durch:</p>
          <OperatorBlock />

          <p className="mt-4">
            Leys Services handelt als Auftragsbearbeiter im Sinne von Art. 9 nDSG nach den Weisungen des Anbieters und
            ist für den technischen Betrieb verantwortlich, insbesondere für Hosting, Datensicherheit, Verfügbarkeit
            sowie die Auswahl und Steuerung der eingesetzten Dienstleister. Der Anbieter bleibt verantwortlich für die
            Inhalte seines Auftritts, sein Sortiment, die Rechtmässigkeit der Datenbearbeitung sowie die Beantwortung
            von Anfragen betroffener Personen. Leys Services verarbeitet zusätzlich bestimmte technische Daten für
            Betrieb und Sicherheit der Plattform (z. B. Server-Protokolle, Authentifizierungs- und Sicherheitsdaten) in
            eigener Verantwortung.
          </p>
          <p className="mt-2">
            Bei Fragen zum Datenschutz können Sie sich an den Anbieter{email ? ` (${email})` : ""} oder an Leys Services
            (contact@leys.ch) wenden.
          </p>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">2. Erhobene Personendaten</h2>
          <p className="mb-2">Wir bearbeiten folgende Kategorien von Personendaten:</p>
          <ul className="list-disc space-y-1 pl-5">
            <li>
              <strong>Kontodaten:</strong> Name, E-Mail-Adresse, Google-Konto-Informationen bei Anmeldung via Google
              (die Anmeldung erfolgt per E-Mail-Einmalcode oder über Google, wir speichern kein Passwort)
            </li>
            <li>
              <strong>Bestelldaten:</strong> Bestellte Artikel, Bestellverlauf, Zeitstempel, Bestellstatus
            </li>
            <li>
              <strong>Zahlungsdaten:</strong> Zahlungsmethode, Transaktions-IDs und Zahlungsstatus (Payrexx wickelt die
              eigentliche Zahlung ab; wir speichern keine Kreditkarten- oder TWINT-Kontodaten)
            </li>
            <li>
              <strong>Technische Daten:</strong> IP-Adresse, Browsertyp, Betriebssystem, Gerätetyp,
              Sitzungsinformationen
            </li>
            <li>
              <strong>Kommunikationsdaten:</strong> Inhalte von E-Mails, die im Zusammenhang mit Bestellungen versendet
              werden (z.B. Bestellbestätigungen, Quittungen)
            </li>
          </ul>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">3. Zwecke der Datenbearbeitung</h2>
          <p className="mb-2">Wir bearbeiten Ihre Personendaten zu folgenden Zwecken:</p>
          <ul className="list-disc space-y-1 pl-5">
            <li>Erstellung und Verwaltung Ihres Benutzerkontos</li>
            <li>Abwicklung und Erfüllung Ihrer Bestellungen</li>
            <li>Zahlungsabwicklung über Payrexx (TWINT)</li>
            <li>Versand von transaktionalen E-Mails (Bestellbestätigungen, Quittungen)</li>
            <li>Sitzungsverwaltung und Authentifizierung</li>
            <li>Gewährleistung der Sicherheit und Funktionalität der Plattform</li>
            <li>Erfüllung gesetzlicher Aufbewahrungspflichten</li>
          </ul>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">4. Empfänger und Auftragsbearbeiter</h2>
          <p className="mb-2">
            Die Plattform wird von Leys Services betrieben. Für Betrieb, Zahlung und Anmeldung werden folgende
            Dienstleister eingesetzt:
          </p>
          <ul className="list-disc space-y-1 pl-5">
            {SUBPROCESSORS.map((sp) => (
              <li key={sp.name}>
                <strong>{sp.name}</strong> ({sp.location}): {sp.purpose}
              </li>
            ))}
          </ul>
          <p className="mt-2">
            Mit allen Auftragsbearbeitern bestehen Vereinbarungen zur Auftragsbearbeitung gemäss Art. 9 nDSG.
          </p>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">5. Datenübermittlung ins Ausland</h2>
          <p>
            Die Plattform wird in der EU betrieben: Hosting in Deutschland (Hetzner) sowie Datenbank- und Dateispeicher
            in Frankreich (Scaleway, Paris). Der Bundesrat anerkennt den Datenschutz in den EU-/EWR-Staaten als
            angemessen (Art. 16 Abs. 1 nDSG).
          </p>
          <p className="mt-2">Einzelne Dienstleister übermitteln Personendaten ausserdem in folgende Länder:</p>
          <ul className="mt-2 list-disc space-y-1 pl-5">
            {FOREIGN_TRANSFERS.map((t) => (
              <li key={t.country}>
                <strong>{t.country}</strong> ({t.parties}): Die Übermittlung erfolgt auf Grundlage von
                Standarddatenschutzklauseln (Standard Contractual Clauses, SCC) oder anderer geeigneter Garantien gemäss
                Art. 16–17 nDSG.
              </li>
            ))}
          </ul>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">6. Cookies und Sitzungsverwaltung</h2>
          <p>
            Wir verwenden technisch notwendige Cookies für die Sitzungsverwaltung und Authentifizierung. Diese Cookies
            sind für den Betrieb der Plattform erforderlich und ermöglichen es Ihnen, angemeldet zu bleiben und
            Bestellungen aufzugeben. Es werden keine Tracking- oder Werbe-Cookies eingesetzt.
          </p>
          <p className="mt-2">
            Sie können Cookies in Ihren Browsereinstellungen deaktivieren. Beachten Sie jedoch, dass die Plattform ohne
            Cookies nicht vollständig funktionsfähig ist.
          </p>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">7. Aufbewahrungsdauer</h2>
          <p className="mb-2">
            Wir bewahren Ihre Personendaten nur so lange auf, wie es für die genannten Zwecke erforderlich ist:
          </p>
          <ul className="list-disc space-y-1 pl-5">
            <li>
              <strong>Kontodaten:</strong> Bis zur Löschung Ihres Kontos
            </li>
            <li>
              <strong>Bestell- und Zahlungsdaten:</strong> 10 Jahre gemäss gesetzlicher Aufbewahrungspflicht (Art. 958f
              OR)
            </li>
            <li>
              <strong>Technische Daten / Logs:</strong> Maximal 90 Tage
            </li>
          </ul>
          <p className="mt-2">Nach Ablauf der Aufbewahrungsfrist werden Ihre Daten gelöscht oder anonymisiert.</p>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">8. Datensicherheit</h2>
          <p>
            Wir treffen angemessene technische und organisatorische Massnahmen zum Schutz Ihrer Personendaten gemäss
            Art. 8 nDSG. Dazu gehören insbesondere verschlüsselte Datenübertragung (TLS), Zugangskontrollen,
            passwortlose Anmeldung per E-Mail-Einmalcode sowie regelmässige Sicherheitsüberprüfungen.
          </p>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">9. Ihre Rechte</h2>
          <p className="mb-2">Gemäss nDSG stehen Ihnen folgende Rechte zu:</p>
          <ul className="list-disc space-y-1 pl-5">
            <li>
              <strong>Auskunftsrecht (Art. 25 nDSG):</strong> Sie haben das Recht, Auskunft darüber zu erhalten, ob und
              welche Personendaten wir über Sie bearbeiten.
            </li>
            <li>
              <strong>Recht auf Datenherausgabe (Art. 28 nDSG):</strong> Sie können verlangen, dass Ihnen Ihre Daten in
              einem gängigen elektronischen Format herausgegeben werden.
            </li>
            <li>
              <strong>Recht auf Berichtigung:</strong> Sie können die Korrektur unrichtiger Personendaten verlangen.
            </li>
            <li>
              <strong>Recht auf Löschung:</strong> Sie können die Löschung Ihrer Personendaten verlangen, sofern keine
              gesetzlichen Aufbewahrungspflichten entgegenstehen. Sie können Ihr Konto jederzeit in den
              Profileinstellungen löschen.
            </li>
          </ul>
          {email ? (
            <p className="mt-2">
              Zur Ausübung Ihrer Rechte kontaktieren Sie uns bitte per E-Mail an {email}. Wir werden Ihr Anliegen innert
              30 Tagen bearbeiten.
            </p>
          ) : (
            <p className="mt-2">
              Zur Ausübung Ihrer Rechte kontaktieren Sie uns bitte per E-Mail. Wir werden Ihr Anliegen innert 30 Tagen
              bearbeiten.
            </p>
          )}
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">10. Beschwerderecht</h2>
          <p>
            Sie haben das Recht, eine Beschwerde beim Eidgenössischen Datenschutz- und Öffentlichkeitsbeauftragten
            (EDÖB) einzureichen:
          </p>
          <p className="mt-2">
            EDÖB
            <br />
            Feldeggweg 1
            <br />
            3003 Bern
            <br />
            Schweiz
            <br />
            www.edoeb.admin.ch
          </p>
        </section>

        <section>
          <h2 className="mb-3 text-lg font-semibold">11. Änderungen</h2>
          <p>
            Wir behalten uns vor, diese Datenschutzerklärung jederzeit anzupassen. Die aktuelle Fassung ist auf der
            Plattform abrufbar. Bei wesentlichen Änderungen informieren wir Sie über die auf der Plattform angegebene
            E-Mail-Adresse.
          </p>
        </section>
      </div>
    </div>
  )
}
