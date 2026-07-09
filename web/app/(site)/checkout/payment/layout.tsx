import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Zahlung",
  description: "Bezahle deine Bestellung bequem mit TWINT bei Comanda.",
  alternates: { canonical: "/checkout/payment" },
  openGraph: {
    title: "Zahlung",
    description: "TWINT-Zahlung für deine Comanda Bestellung starten.",
    url: "/checkout/payment",
  },
}

export default function PaymentLayout({ children }: { children: React.ReactNode }) {
  return children
}
