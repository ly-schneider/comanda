import { Geist, IBM_Plex_Sans, Inter, JetBrains_Mono, Lora, Space_Grotesk } from "next/font/google"

// Curated, platform-defined fonts. Each exposes a CSS variable; the active one
// is selected per tenant at runtime by repointing --font-family-* (see
// lib/branding-theme.ts buildThemeCss). next/font requires static module-scope
// instantiation, so every option is declared here and all variables are applied
// on <html> via FONT_VARS. Keys/variables MUST match FONT_KEY_TO_VAR.
const inter = Inter({ subsets: ["latin"], variable: "--font-inter", display: "swap" })
const geist = Geist({ subsets: ["latin"], variable: "--font-geist", display: "swap" })
const spaceGrotesk = Space_Grotesk({ subsets: ["latin"], variable: "--font-space-grotesk", display: "swap" })
const ibmPlexSans = IBM_Plex_Sans({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-ibm-plex-sans",
  display: "swap",
})
const jetbrainsMono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-jetbrains-mono", display: "swap" })
const lora = Lora({ subsets: ["latin"], variable: "--font-lora", display: "swap" })

export const FONT_VARS = [inter, geist, spaceGrotesk, ibmPlexSans, jetbrainsMono, lora].map((f) => f.variable).join(" ")
