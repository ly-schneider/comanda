import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Bezahlung fehlgeschlagen",
  description: "Die Zahlung ist fehlgeschlagen – versuche es erneut oder kehre zum Warenkorb zurück.",
  alternates: { canonical: "/checkout/error" },
  openGraph: {
    title: "Bezahlung fehlgeschlagen",
    description: "Zahlung fehlgeschlagen – zurück zum Warenkorb oder erneut versuchen.",
    url: "/checkout/error",
  },
}

export default function CheckoutErrorLayout({ children }: { children: React.ReactNode }) {
  return children
}
