import { PLATFORM_OPERATOR } from "@/lib/legal"

export const CONTACT_EMAIL = PLATFORM_OPERATOR.email
export const CONTACT_MAILTO = `mailto:${CONTACT_EMAIL}?subject=Comanda`

export const LINKS = {
  blog: "https://www.leys.ch/blog",
  blogFirst: "https://www.leys.ch/blog/building-a-checkout-system-at-16",
  blogSecond: "https://www.leys.ch/blog/the-checkout-system-got-a-second-life",
  leys: "https://www.leys.ch",
  license: "https://github.com/ly-schneider/comanda/blob/main/LICENSE",
} as const

export const SECTIONS = {
  howItWorks: "so-funktionierts",
  inUse: "im-einsatz",
  story: "geschichte",
  contact: "kontakt",
} as const

export const NAV_LINKS = [
  { label: "So funktioniert's", href: `/#${SECTIONS.howItWorks}` },
  { label: "Im Einsatz", href: `/#${SECTIONS.inUse}` },
  { label: "Geschichte", href: `/#${SECTIONS.story}` },
  { label: "Kontakt", href: `/#${SECTIONS.contact}` },
] as const

export const LEGAL_DOCS = [
  { label: "Impressum", href: "/impressum" },
  { label: "Datenschutz", href: "/datenschutz" },
  { label: "Lizenz", href: "/lizenz" },
] as const
