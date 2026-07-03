import "styles/tailwind.css"
import type { Metadata } from "next"
import AnalyticsWrapper from "@/components/analytics-wrapper"
import { AuthProvider } from "@/contexts/auth-context"
import { TenantBrandingProvider } from "@/contexts/branding-context"
import { buildThemeCss } from "@/lib/branding-theme"
import { FONT_VARS } from "@/lib/fonts"
import { getBranding, isTenantSuspended } from "@/lib/server/branding"

export const dynamic = "force-dynamic"

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

  if (await isTenantSuspended()) {
    return (
      <html lang="de" className={FONT_VARS}>
        <body>
          <main className="flex min-h-screen flex-col items-center justify-center gap-2 px-6 text-center">
            <h1 className="text-2xl font-semibold">Vorübergehend nicht verfügbar</h1>
            <p className="text-muted-foreground max-w-md">
              Dieser Anbieter ist zurzeit deaktiviert. Bitte versuche es später erneut.
            </p>
          </main>
        </body>
      </html>
    )
  }

  // Per-tenant accents, font, and corner radii injected over the fixed white
  // base. Values are backend-validated hex / allow-listed keys (see buildThemeCss).
  const themeStyle = buildThemeCss(branding)

  return (
    <html lang="de" className={FONT_VARS}>
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
