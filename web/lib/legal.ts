// Fixed identity of the platform operator. Unlike tenant branding (which varies
// per tenant), this is identical on every storefront: Leys Services runs the
// infrastructure as the tenant's Auftragsbearbeiter (processor) under the nDSG.
export const PLATFORM_OPERATOR = {
  legalName: "Leys Services",
  owner: "Levyn Schneider",
  addressLines: ["Neumattstrasse 26", "3127 Mühlethurnen"],
  country: "Schweiz",
  email: "contact@leys.ch",
} as const

export type SubProcessor = { name: string; location: string; purpose: string }

// Providers engaged to run the platform, process payments, and sign in users.
// Hosting and data storage stay in the EU; the USA entries drive the cross-border
// transfer disclosure in the privacy policy.
export const SUBPROCESSORS: readonly SubProcessor[] = [
  { name: "Hetzner Online GmbH", location: "Deutschland (EU)", purpose: "Hosting und Server-Infrastruktur" },
  { name: "Scaleway SAS", location: "Frankreich, Paris (EU)", purpose: "Datenbank sowie Datei- und Objektspeicher" },
  { name: "Cloudflare, Inc.", location: "USA", purpose: "CDN und Proxy für die Auslieferung von Inhalten" },
  { name: "Sentry", location: "EU (EU-Datenhaltung)", purpose: "Fehler- und Betriebsüberwachung der Plattform" },
  { name: "Payrexx AG", location: "Thun, Schweiz", purpose: "Zahlungsabwicklung (TWINT)" },
  { name: "Plunk (Amazon SES)", location: "USA", purpose: "Versand transaktionaler E-Mails" },
  { name: "Google LLC", location: "USA", purpose: "Anmeldung via Google (nur bei Wahl dieser Anmeldemethode)" },
] as const

// Countries without a Federal-Council adequacy decision that data may reach via
// the providers above. EU/EEA transfers are not listed (recognised as adequate).
export const FOREIGN_TRANSFERS = [{ country: "USA", parties: "Google, Plunk (Amazon SES), Cloudflare" }] as const
