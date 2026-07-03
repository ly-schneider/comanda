import { describe, expect, it } from "vitest"
import { qaExportFilename, qaRunToCsv, qaRunToMarkdown } from "./qa-export"
import { QA_TEST_COUNT } from "./qa-tests"

const results = {
  "smoke.health": { status: "success" as const, note: "" },
  "smoke.platform-admin-gate": { status: "error" as const, note: 'Ticket "LEY-999", siehe | Notiz\nZeile 2' },
}

describe("qaRunToCsv", () => {
  it("emits a header plus one row per test", () => {
    const lines = qaRunToCsv(results).split("\n")
    expect(lines[0]).toBe("Abschnitt,Key,Test,Status,Notiz")
    // The quoted note contains a newline, so raw lines = tests + header + 1.
    expect(lines.length).toBe(QA_TEST_COUNT + 2)
  })

  it("escapes quotes, commas and newlines", () => {
    const csv = qaRunToCsv(results)
    expect(csv).toContain('"Ticket ""LEY-999"", siehe | Notiz\nZeile 2"')
  })
})

describe("qaRunToMarkdown", () => {
  it("renders summary, section headings and status symbols", () => {
    const md = qaRunToMarkdown({ name: "Pre-Launch", createdAt: "2026-07-03T10:00:00Z" }, results)
    expect(md).toContain("# QA-Testlauf: Pre-Launch")
    expect(md).toContain(`Erfasst: 2/${QA_TEST_COUNT} · 1 ✓ · 1 ✗`)
    expect(md).toContain("## 0 · Plattform-Smoke & Zugriff (2/3 · 1 ✓ · 1 ✗)")
    expect(md).toContain("| ✓ | Health-Endpunkte antworten |")
  })

  it("escapes pipes and newlines in notes", () => {
    const md = qaRunToMarkdown({ name: "x", createdAt: "2026-07-03T10:00:00Z" }, results)
    expect(md).toContain('Ticket "LEY-999", siehe \\| Notiz<br>Zeile 2')
  })
})

describe("qaExportFilename", () => {
  it("slugifies the run name", () => {
    expect(qaExportFilename("Staging Pre-Launch (Küche) 2026", "csv")).toBe("qa-staging-pre-launch-kueche-2026.csv")
    expect(qaExportFilename("///", "md")).toBe("qa-testlauf.md")
  })
})
