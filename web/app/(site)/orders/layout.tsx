import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Bestellungen",
  description: "Übersicht deiner Bestellungen und QR-Codes zur Abholung bei Comanda.",
  alternates: { canonical: "/orders" },
  openGraph: {
    title: "Bestellungen",
    description: "Deine Bestellungen und Abholungs-QR-Codes verwalten.",
    url: "/orders",
  },
}

export default function OrdersLayout({ children }: { children: React.ReactNode }) {
  return <div className="mx-auto max-w-xl">{children}</div>
}
