import { FOREIGN_TRANSFERS, PLATFORM_OPERATOR, SUBPROCESSORS } from "@/lib/legal"
import { LegalList, LegalPage, LegalRows, LegalText } from "./legal-page"

// Payrexx only processes storefront payments; visitors of the platform site never pay here.
const SITE_PROCESSORS = SUBPROCESSORS.filter((sp) => !sp.name.startsWith("Payrexx"))

const mail = (
  <a href={`mailto:${PLATFORM_OPERATOR.email}`} className="underline underline-offset-4">
    {PLATFORM_OPERATOR.email}
  </a>
)

export function PlatformPrivacy() {
  const [street, city] = PLATFORM_OPERATOR.addressLines
  return (
    <LegalPage
      doc="Datenschutz"
      lead="Welche Daten anfallen, wenn du diese Website besuchst, mir schreibst oder dich anmeldest, und was damit passiert."
      meta="Nach dem Schweizer Datenschutzgesetz (nDSG) · Stand: 11. November 2026"
      sections={[
        {
          id: "verantwortlich",
          title: "Wer verantwortlich ist",
          content: (
            <>
              <LegalRows
                rows={[
                  ["Verantwortlich", `${PLATFORM_OPERATOR.legalName}, ${PLATFORM_OPERATOR.owner}`],
                  ["Adresse", street, `${city}, ${PLATFORM_OPERATOR.country}`],
                  ["E-Mail", mail],
                ]}
              />
              <LegalText>
                Diese Erklärung gilt für comanda.leys.ch. Bestellst du an einem Stand, ist der jeweilige Veranstalter
                für deine Bestelldaten verantwortlich. Seine Datenschutzerklärung findest du in seinem Bestellshop.
              </LegalText>
            </>
          ),
        },
        {
          id: "daten",
          title: "Welche Daten anfallen",
          content: (
            <LegalList
              items={[
                "Beim Besuch der Website: IP-Adresse, Browser, Betriebssystem, Zeitpunkt und aufgerufene Seite. Das sind technische Protokolle, die jeder Server schreibt.",
                "Wenn du mir schreibst: deine E-Mail-Adresse und was du mir mitteilst.",
                "Wenn du dich anmeldest: Name und E-Mail-Adresse, bei der Anmeldung mit Google auch die Angaben aus deinem Google-Konto. Ein Passwort gibt es nicht, du bekommst einen Einmalcode per E-Mail.",
              ]}
            />
          ),
        },
        {
          id: "zweck",
          title: "Wofür ich sie brauche",
          content: (
            <>
              <LegalList
                items={[
                  "um die Website auszuliefern und vor Angriffen zu schützen,",
                  "um deine Anfrage zu beantworten,",
                  "um dich anzumelden und dein Konto zu verwalten,",
                  "um Fehler zu finden und zu beheben.",
                ]}
              />
              <LegalText>Ich verkaufe keine Daten und nutze sie nicht für Werbung.</LegalText>
            </>
          ),
        },
        {
          id: "statistik",
          title: "Statistik ohne Cookies",
          content: (
            <LegalText>
              Um zu sehen, wie viele Leute die Website besuchen und welche Seiten sie lesen, nutze ich Umami. Umami
              setzt keine Cookies und speichert keine Personendaten. Einzelne Besucherinnen und Besucher lassen sich
              damit nicht erkennen.
            </LegalText>
          ),
        },
        {
          id: "dienstleister",
          title: "Wer sonst noch beteiligt ist",
          content: (
            <>
              <LegalText>
                Für den Betrieb arbeite ich mit diesen Dienstleistern zusammen. Mit allen bestehen Verträge zur
                Auftragsbearbeitung nach Art. 9 nDSG.
              </LegalText>
              <LegalRows
                labelWidth="sm:w-[200px]"
                rows={SITE_PROCESSORS.map((sp) => [sp.name, sp.purpose, sp.location])}
              />
            </>
          ),
        },
        {
          id: "ausland",
          title: "Daten im Ausland",
          content: (
            <>
              <LegalText>
                Die Website läuft in der EU, mit Servern in Deutschland und der Datenbank in Paris. Den Datenschutz in
                der EU anerkennt der Bundesrat als angemessen (Art. 16 Abs. 1 nDSG).
              </LegalText>
              {FOREIGN_TRANSFERS.map((t) => (
                <LegalText key={t.country}>
                  {t.parties} können Daten in die {t.country} übermitteln. Das geschieht auf Grundlage von
                  Standardvertragsklauseln oder anderen geeigneten Garantien nach Art. 16 und 17 nDSG.
                </LegalText>
              ))}
            </>
          ),
        },
        {
          id: "cookies",
          title: "Cookies",
          content: (
            <LegalText>
              Ich setze nur Cookies, die für die Anmeldung nötig sind. Tracking- oder Werbe-Cookies gibt es keine,
              deshalb auch keinen Cookie-Banner.
            </LegalText>
          ),
        },
        {
          id: "aufbewahrung",
          title: "Wie lange ich Daten aufbewahre",
          content: (
            <>
              <LegalRows
                rows={[
                  ["Server-Protokolle", "höchstens 90 Tage"],
                  ["E-Mails an mich", "so lange, wie ich sie für deine Anfrage brauche"],
                  ["Kontodaten", "bis du dein Konto löschst"],
                ]}
              />
              <LegalText>Danach lösche oder anonymisiere ich die Daten.</LegalText>
            </>
          ),
        },
        {
          id: "sicherheit",
          title: "Sicherheit",
          content: (
            <LegalText>
              Alle Verbindungen sind verschlüsselt (TLS). Der Zugriff auf Daten ist auf das Nötige beschränkt, und die
              Anmeldung funktioniert ohne Passwort, das gestohlen werden könnte.
            </LegalText>
          ),
        },
        {
          id: "rechte",
          title: "Deine Rechte",
          content: (
            <>
              <LegalText>Du kannst jederzeit verlangen,</LegalText>
              <LegalList
                items={[
                  "zu erfahren, welche Daten ich über dich bearbeite (Art. 25 nDSG),",
                  "deine Daten in einem gängigen Format zu bekommen (Art. 28 nDSG),",
                  "falsche Daten zu berichtigen,",
                  "deine Daten zu löschen, soweit keine gesetzliche Aufbewahrungspflicht besteht.",
                ]}
              />
              <LegalText>Eine kurze E-Mail an {mail} genügt. Ich antworte innert 30 Tagen.</LegalText>
            </>
          ),
        },
        {
          id: "beschwerde",
          title: "Beschwerde",
          content: (
            <>
              <LegalText>
                Bist du nicht einverstanden, wie ich mit deinen Daten umgehe, kannst du dich beim Eidgenössischen
                Datenschutz- und Öffentlichkeitsbeauftragten beschweren:
              </LegalText>
              <LegalRows
                rows={[
                  [
                    "EDÖB",
                    "Feldeggweg 1",
                    <a key="edoeb" href="https://www.edoeb.admin.ch">
                      3003 Bern, Schweiz · edoeb.admin.ch
                    </a>,
                  ],
                ]}
              />
            </>
          ),
        },
        {
          id: "aenderungen",
          title: "Änderungen",
          content: (
            <LegalText>
              Ändert sich etwas an dieser Erklärung, steht hier oben das neue Datum. Bei wesentlichen Änderungen für
              angemeldete Konten informiere ich per E-Mail.
            </LegalText>
          ),
        },
      ]}
    />
  )
}
