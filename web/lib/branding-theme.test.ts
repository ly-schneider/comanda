import { describe, expect, it } from "vitest"
import { buildThemeCss, FONT_KEY_TO_VAR, FONT_KEYS, pickForeground, RADIUS_KEYS } from "@/lib/branding-theme"
import type { Branding } from "@/lib/server/branding"

// Mirror of the backend allow-lists (allowedFontKeys / allowedRadiusKeys in
// backend/internal/service/settings.go). If these drift, the admin picker can
// offer a value the backend rejects (or vice-versa).
const BACKEND_FONT_KEYS = ["inter", "geist", "space-grotesk", "ibm-plex-sans", "jetbrains-mono", "lora"]
const BACKEND_RADIUS_KEYS = ["none", "rounded", "pill"]

function branding(overrides: Partial<Branding> = {}): Branding {
  return {
    displayName: "Test",
    logoUrl: null,
    faviconUrl: null,
    colors: {
      primary: "#0a0a0a",
      primaryForeground: "#ffffff",
      secondary: "#f5f5f5",
      background: "#ffffff",
      foreground: "#0a0a0a",
    },
    fontKey: "inter",
    radiusKey: "rounded",
    contact: { email: null, phone: null },
    legal: { name: null, address: null },
    currency: "CHF",
    ...overrides,
  }
}

describe("branding-theme allow-lists", () => {
  it("font keys match the backend allow-list", () => {
    expect([...FONT_KEYS].sort()).toEqual([...BACKEND_FONT_KEYS].sort())
  })
  it("every font key maps to a --font- css variable", () => {
    for (const k of FONT_KEYS) expect(FONT_KEY_TO_VAR[k]).toMatch(/^--font-/)
  })
  it("radius keys match the backend allow-list", () => {
    expect([...RADIUS_KEYS].sort()).toEqual([...BACKEND_RADIUS_KEYS].sort())
  })
})

describe("pickForeground", () => {
  it("returns light text on dark backgrounds", () => {
    expect(pickForeground("#0a0a0a")).toBe("#ffffff")
    expect(pickForeground("#1d4ed8")).toBe("#ffffff")
  })
  it("returns dark text on light backgrounds", () => {
    expect(pickForeground("#ffffff")).toBe("#0a0a0a")
    expect(pickForeground("#f5f5f5")).toBe("#0a0a0a")
  })
})

describe("buildThemeCss", () => {
  it("injects the chosen accents, font, and radii", () => {
    const css = buildThemeCss(
      branding({
        colors: { ...branding().colors, primary: "#ff8800", secondary: "#1122ff" },
        fontKey: "geist",
        radiusKey: "pill",
      })
    )
    expect(css).toContain("--primary: #ff8800;")
    expect(css).toContain("--secondary: #1122ff;")
    expect(css).toContain("var(--font-geist)")
    expect(css).toContain("--control-radius: 9999px;")
    expect(css).toContain("--cta-radius: 9999px;")
  })
  it("falls back to defaults for unknown keys", () => {
    const css = buildThemeCss(branding({ fontKey: "bogus", radiusKey: "bogus" }))
    expect(css).toContain("var(--font-inter)")
    expect(css).toContain("--control-radius: 0.625rem;")
  })
  it("computes a readable foreground for a light accent", () => {
    const css = buildThemeCss(branding({ colors: { ...branding().colors, primary: "#ffffff" } }))
    expect(css).toContain("--primary-foreground: #0a0a0a;")
  })
})
