import "styles/tailwind.css"
import type { Metadata } from "next"
import { Golos_Text } from "next/font/google"
import AnalyticsWrapper from "@/components/analytics-wrapper"
import { AuthProvider } from "@/contexts/auth-context"
import { TenantBrandingProvider } from "@/contexts/branding-context"
import { getBranding } from "@/lib/server/branding"

export const dynamic = "force-dynamic"

const golosText = Golos_Text({
  weight: ["500"],
  subsets: ["latin"],
  variable: "--font-golos-text",
})

// noindex for staging/dev is set at runtime via X-Robots-Tag in proxy.ts.
export async function generateMetadata(): Promise<Metadata> {
  const branding = await getBranding()
  return {
    title: {
      default: branding.displayName,
      template: `%s - ${branding.displayName}`,
    },
    description: `Das Food-Bestellsystem von ${branding.displayName} – mit TWINT bezahlen und abholen.`,
    openGraph: {
      siteName: branding.displayName,
      type: "website",
      locale: "de_CH",
    },
    ...(branding.faviconUrl
      ? { icons: { icon: branding.faviconUrl, shortcut: branding.faviconUrl } }
      : { icons: { icon: "/favicon.ico", shortcut: "/favicon.ico" } }),
    ...(process.env.NEXT_PUBLIC_GOOGLE_SITE_VERIFICATION
      ? { verification: { google: process.env.NEXT_PUBLIC_GOOGLE_SITE_VERIFICATION } }
      : {}),
  }
}

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const branding = await getBranding()
  const { primary, primaryForeground, background, foreground } = branding.colors

  // Hex color values from backend are validated (/^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/)
  // so interpolating them into a CSS string is safe.
  const themeStyle = [
    ":root,",
    ".dark {",
    `  --primary: ${primary};`,
    `  --primary-hover: ${primary};`,
    `  --primary-foreground: ${primaryForeground};`,
    `  --background: ${background};`,
    `  --foreground: ${foreground};`,
    `  --ring: ${primary};`,
    `  --chart-1: ${primary};`,
    `  --sidebar-accent: ${primary};`,
    `  --sidebar-accent-foreground: ${primaryForeground};`,
    `  --sidebar-ring: ${primary};`,
    "}",
  ].join("\n")

  return (
    <html lang="de" className={golosText.variable}>
      <head>
        <style dangerouslySetInnerHTML={{ __html: themeStyle }} />
      </head>
      <body>
        <TenantBrandingProvider branding={branding}>
          <AuthProvider>{children}</AuthProvider>
        </TenantBrandingProvider>
        <AnalyticsWrapper />
      </body>
    </html>
  )
}
