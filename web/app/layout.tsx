import "styles/tailwind.css"
import type { Metadata } from "next"
import { Golos_Text } from "next/font/google"
import AnalyticsWrapper from "@/components/analytics-wrapper"
import { AuthProvider } from "@/contexts/auth-context"

const golosText = Golos_Text({
  weight: ["500"],
  subsets: ["latin"],
  variable: "--font-golos-text",
})

export const metadata: Metadata = {
  title: {
    default: "Comanda",
    template: "%s - Comanda",
  },
  description: "Das Food-Bestellsystem von Comanda – mit TWINT bezahlen und abholen.",
  openGraph: {
    siteName: "Comanda",
    type: "website",
    locale: "de_CH",
  },
  robots: {
    index: true,
    follow: true,
  },
  verification: {
    google: "BHYX2MGysKIAqHYfd-nyBvfNhP-3zImiovPfzGSQEjc",
  },
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="de" className={golosText.variable}>
      <body>
        <AuthProvider>{children}</AuthProvider>
        <AnalyticsWrapper />
      </body>
    </html>
  )
}
