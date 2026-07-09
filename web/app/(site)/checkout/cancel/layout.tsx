import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Bezahlung abgebrochen",
  description: "Die Zahlung wurde abgebrochen – versuche es erneut oder kehre zum Warenkorb zurück.",
  alternates: { canonical: "/checkout/cancel" },
  openGraph: {
    title: "Bezahlung abgebrochen",
    description: "Zahlung nicht abgeschlossen – zurück zum Warenkorb oder erneut versuchen.",
    url: "/checkout/cancel",
  },
}

export default function CheckoutCancelLayout({ children }: { children: React.ReactNode }) {
  return children
}
