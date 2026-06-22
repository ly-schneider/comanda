import type { Branding } from "@/lib/server/branding"

// Curated font set. Keys MUST match the backend allow-list (allowedFontKeys in
// backend/internal/service/settings.go) and the next/font instances in
// lib/fonts.ts. A key maps to a CSS variable name; the raw key never reaches CSS.
export const DEFAULT_FONT_KEY = "inter"

export const FONT_OPTIONS: { key: string; label: string }[] = [
  { key: "inter", label: "Inter" },
  { key: "geist", label: "Geist" },
  { key: "space-grotesk", label: "Space Grotesk" },
  { key: "ibm-plex-sans", label: "IBM Plex Sans" },
  { key: "jetbrains-mono", label: "JetBrains Mono" },
  { key: "lora", label: "Lora (Serif)" },
]

export const FONT_KEYS = FONT_OPTIONS.map((o) => o.key)

export const FONT_KEY_TO_VAR: Record<string, string> = {
  inter: "--font-inter",
  geist: "--font-geist",
  "space-grotesk": "--font-space-grotesk",
  "ibm-plex-sans": "--font-ibm-plex-sans",
  "jetbrains-mono": "--font-jetbrains-mono",
  lora: "--font-lora",
}

// Corner style presets. Keys match the backend allow-list (allowedRadiusKeys).
// `none` squares everything (incl. the primary CTA); `rounded` is the modest
// default; `pill` makes controls fully round (cards/dialogs stay moderate).
export const DEFAULT_RADIUS_KEY = "rounded"

export const RADIUS_OPTIONS: { key: string; label: string }[] = [
  { key: "none", label: "Eckig" },
  { key: "rounded", label: "Abgerundet" },
  { key: "pill", label: "Pill" },
]

export const RADIUS_KEYS = RADIUS_OPTIONS.map((o) => o.key)

type RadiusValues = { control: string; surface: string; cta: string }

export const RADIUS_KEY_TO_VALUES: Record<string, RadiusValues> = {
  none: { control: "0px", surface: "0px", cta: "0px" },
  rounded: { control: "0.625rem", surface: "1rem", cta: "9999px" },
  pill: { control: "9999px", surface: "1rem", cta: "9999px" },
}

const HEX_RE = /^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/

function safeColor(value: string, fallback: string): string {
  return HEX_RE.test(value) ? value : fallback
}

// pickForeground returns the readable text color (near-black or white) for a
// given background hex, so accent buttons/badges stay legible whatever color a
// tenant picks. Uses perceptual luminance.
export function pickForeground(hex: string): string {
  const m = hex.trim().match(/^#?([0-9a-fA-F]{6})/)
  const hex6 = m?.[1]
  if (!hex6) return "#ffffff"
  const int = parseInt(hex6, 16)
  const r = (int >> 16) & 0xff
  const g = (int >> 8) & 0xff
  const b = int & 0xff
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255
  return luminance > 0.6 ? "#0a0a0a" : "#ffffff"
}

// buildThemeCss renders the per-tenant :root overrides injected into <head>.
// It overrides only the accent colors, the active font, and the corner radii —
// the white/monochrome base (background, foreground, cards, borders) stays
// fixed. Colors are backend-validated hex; font/radius come from allow-listed
// keys mapped to constants here, so nothing untrusted is interpolated into CSS.
export function buildThemeCss(branding: Branding): string {
  const primary = safeColor(branding.colors.primary, "#0a0a0a")
  const secondary = safeColor(branding.colors.secondary, "#f5f5f5")
  const primaryForeground = pickForeground(primary)
  const secondaryForeground = pickForeground(secondary)
  const fontVar = FONT_KEY_TO_VAR[branding.fontKey] ?? "--font-inter"
  const radius = RADIUS_KEY_TO_VALUES[branding.radiusKey] ?? { control: "0.625rem", surface: "1rem", cta: "9999px" }
  const fontFamily = `var(${fontVar}), ui-sans-serif, system-ui, sans-serif`

  return [
    ":root {",
    `  --primary: ${primary};`,
    `  --primary-hover: ${primary};`,
    `  --primary-foreground: ${primaryForeground};`,
    `  --secondary: ${secondary};`,
    `  --secondary-hover: ${secondary};`,
    `  --secondary-foreground: ${secondaryForeground};`,
    `  --ring: ${primary};`,
    `  --chart-1: ${primary};`,
    `  --sidebar-accent: ${primary};`,
    `  --sidebar-accent-foreground: ${primaryForeground};`,
    `  --sidebar-ring: ${primary};`,
    `  --font-family-primary: ${fontFamily};`,
    `  --font-family-secondary: ${fontFamily};`,
    `  --control-radius: ${radius.control};`,
    `  --surface-radius: ${radius.surface};`,
    `  --cta-radius: ${radius.cta};`,
    "}",
  ].join("\n")
}
