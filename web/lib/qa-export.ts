import { QA_SECTIONS, QA_TEST_COUNT, type QaStatus } from "./qa-tests"

export type QaExportRun = { name: string; createdAt: string }
export type QaExportResult = { status: QaStatus | null; note: string }
export type QaExportResults = Record<string, QaExportResult>

const STATUS_SYMBOL: Record<QaStatus, string> = { success: "✓", error: "✗" }

function csvField(value: string): string {
  return /[",\n\r]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value
}

export function qaRunToCsv(results: QaExportResults): string {
  const rows: string[][] = [["Abschnitt", "Key", "Test", "Status", "Notiz"]]
  for (const section of QA_SECTIONS) {
    for (const test of section.tests) {
      const result = results[test.key]
      rows.push([section.title, test.key, test.title, result?.status ?? "", result?.note ?? ""])
    }
  }
  return rows.map((row) => row.map(csvField).join(",")).join("\n")
}

function mdCell(value: string): string {
  return value.replace(/\|/g, "\\|").replace(/\r?\n/g, "<br>")
}

export function qaRunToMarkdown(run: QaExportRun, results: QaExportResults): string {
  let success = 0
  let errors = 0
  for (const result of Object.values(results)) {
    if (result.status === "success") success++
    if (result.status === "error") errors++
  }

  const lines: string[] = [
    `# QA-Testlauf: ${run.name}`,
    "",
    `Erstellt: ${new Date(run.createdAt).toLocaleDateString("de-CH", { dateStyle: "medium" })} · ` +
      `Erfasst: ${success + errors}/${QA_TEST_COUNT} · ${success} ✓ · ${errors} ✗`,
  ]

  for (const section of QA_SECTIONS) {
    let sectionSuccess = 0
    let sectionErrors = 0
    for (const test of section.tests) {
      const status = results[test.key]?.status
      if (status === "success") sectionSuccess++
      if (status === "error") sectionErrors++
    }
    lines.push(
      "",
      `## ${section.title} (${sectionSuccess + sectionErrors}/${section.tests.length} · ${sectionSuccess} ✓ · ${sectionErrors} ✗)`,
      "",
      "| Status | Test | Notiz |",
      "| --- | --- | --- |"
    )
    for (const test of section.tests) {
      const result = results[test.key]
      const symbol = result?.status ? STATUS_SYMBOL[result.status] : "–"
      lines.push(`| ${symbol} | ${mdCell(test.title)} | ${mdCell(result?.note ?? "")} |`)
    }
  }
  lines.push("")
  return lines.join("\n")
}

export function qaExportFilename(runName: string, extension: "csv" | "md"): string {
  const slug =
    runName
      .toLowerCase()
      .replace(/[äöü]/g, (c) => ({ ä: "ae", ö: "oe", ü: "ue" })[c] ?? c)
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "testlauf"
  return `qa-${slug}.${extension}`
}
