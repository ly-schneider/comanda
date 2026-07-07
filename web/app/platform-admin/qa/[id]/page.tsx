"use client"

import { ChevronDown, ChevronRight, Download } from "lucide-react"
import Link from "next/link"
import { useParams } from "next/navigation"
import { useCallback, useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { Textarea } from "@/components/ui/textarea"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"
import { qaExportFilename, qaRunToCsv, qaRunToMarkdown } from "@/lib/qa-export"
import { QA_SECTIONS, QA_TEST_COUNT, type QaStatus } from "@/lib/qa-tests"
import { cn } from "@/lib/utils"

type QaRun = {
  id: string
  name: string
  note?: string
  successCount: number
  errorCount: number
  createdAt: string
}

type QaResult = { testKey: string; status?: QaStatus; note?: string }

type ResultState = { status: QaStatus | null; note: string }

const STATUS_OPTIONS: { value: QaStatus | null; label: string; title: string }[] = [
  { value: null, label: "–", title: "Nicht getestet" },
  { value: "success", label: "✓", title: "Erfolgreich" },
  { value: "error", label: "✗", title: "Fehler" },
]

export default function QaRunPage() {
  const params = useParams<{ id: string }>()
  const runId = params?.id
  const fetchAuth = useAuthorizedFetch()

  const [run, setRun] = useState<QaRun | null>(null)
  const [results, setResults] = useState<Record<string, ResultState>>({})
  const [noteDrafts, setNoteDrafts] = useState<Record<string, string>>({})
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!runId) return
    try {
      const res = await fetchAuth(`/api/v1/platform/qa/runs/${runId}`)
      if (!res.ok) throw new Error(`Error ${res.status}`)
      const data = (await res.json()) as { run: QaRun; results: QaResult[] }
      setRun(data.run)
      const map: Record<string, ResultState> = {}
      for (const r of data.results ?? []) {
        map[r.testKey] = { status: r.status ?? null, note: r.note ?? "" }
      }
      setResults(map)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
    } finally {
      setLoading(false)
    }
  }, [fetchAuth, runId])

  useEffect(() => {
    load()
  }, [load])

  const saveResult = useCallback(
    async (testKey: string, next: ResultState) => {
      if (!runId) return
      const prev = results[testKey] ?? { status: null, note: "" }
      setResults((map) => ({ ...map, [testKey]: next }))
      setError(null)
      try {
        const res = await fetchAuth(`/api/v1/platform/qa/runs/${runId}/results/${testKey}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
          body: JSON.stringify({
            status: next.status ?? undefined,
            note: next.note.trim() ? next.note : undefined,
          }),
        })
        if (!res.ok) throw new Error(`Error ${res.status}`)
      } catch (e) {
        setResults((map) => ({ ...map, [testKey]: prev }))
        setError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
      }
    },
    [fetchAuth, runId, results]
  )

  function setStatus(testKey: string, status: QaStatus | null) {
    const current = results[testKey] ?? { status: null, note: "" }
    if (current.status === status) return
    saveResult(testKey, { ...current, status })
  }

  function noteValue(testKey: string) {
    return noteDrafts[testKey] ?? results[testKey]?.note ?? ""
  }

  function commitNote(testKey: string) {
    const draft = noteDrafts[testKey]
    if (draft === undefined) return
    const current = results[testKey] ?? { status: null, note: "" }
    setNoteDrafts(({ [testKey]: _, ...rest }) => rest)
    if (draft === current.note) return
    saveResult(testKey, { ...current, note: draft })
  }

  const sectionStats = useMemo(() => {
    const stats: Record<string, { success: number; error: number; total: number }> = {}
    for (const section of QA_SECTIONS) {
      let success = 0
      let errors = 0
      for (const test of section.tests) {
        const status = results[test.key]?.status
        if (status === "success") success++
        if (status === "error") errors++
      }
      stats[section.key] = { success, error: errors, total: section.tests.length }
    }
    return stats
  }, [results])

  function exportRun(format: "csv" | "md") {
    if (!run) return
    // BOM so Excel reads the UTF-8 umlauts correctly.
    const content = format === "csv" ? "\uFEFF" + qaRunToCsv(results) : qaRunToMarkdown(run, results)
    const blob = new Blob([content], {
      type: format === "csv" ? "text/csv;charset=utf-8" : "text/markdown;charset=utf-8",
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = qaExportFilename(run.name, format)
    a.click()
    URL.revokeObjectURL(url)
  }

  const totals = useMemo(() => {
    let success = 0
    let errors = 0
    for (const state of Object.values(results)) {
      if (state.status === "success") success++
      if (state.status === "error") errors++
    }
    return { success, error: errors, done: success + errors }
  }, [results])

  if (loading) {
    return <p className="text-muted-foreground text-sm">Laden…</p>
  }
  if (!run) {
    return (
      <div className="space-y-4">
        <p className="text-destructive text-sm">{error ?? "Testlauf nicht gefunden."}</p>
        <Button asChild variant="outline" size="sm">
          <Link href="/platform-admin/qa">Zurück zur Übersicht</Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <Link href="/platform-admin/qa" className="text-muted-foreground text-sm hover:underline">
          ← Alle Testläufe
        </Link>
        <div className="mt-1 flex flex-wrap items-end justify-between gap-2">
          <h1 className="font-primary text-2xl font-semibold">{run.name}</h1>
          <div className="flex items-center gap-3">
            <p className="text-muted-foreground text-sm">
              {totals.done}/{QA_TEST_COUNT} erfasst · {totals.success} ✓
              {totals.error > 0 && <span className="text-destructive"> · {totals.error} ✗</span>}
            </p>
            <Button variant="outline" size="sm" onClick={() => exportRun("csv")}>
              <Download className="h-4 w-4" /> CSV
            </Button>
            <Button variant="outline" size="sm" onClick={() => exportRun("md")}>
              <Download className="h-4 w-4" /> Markdown
            </Button>
          </div>
        </div>
        <Progress value={(totals.done / QA_TEST_COUNT) * 100} className="mt-3" />
      </div>

      {error && (
        <div
          role="alert"
          className="text-destructive bg-destructive/10 rounded-[var(--radius-control)] px-3 py-2 text-sm"
        >
          {error}
        </div>
      )}

      <nav className="flex flex-wrap gap-2">
        {QA_SECTIONS.map((section) => {
          const stats = sectionStats[section.key] ?? {
            success: 0,
            error: 0,
            total: section.tests.length,
          }
          const complete = stats.success + stats.error >= stats.total
          return (
            <a
              key={section.key}
              href={`#${section.key}`}
              className={cn(
                "border-border rounded-full border px-3 py-1 text-xs",
                stats.error > 0
                  ? "border-destructive/50 text-destructive"
                  : complete
                    ? "border-emerald-600/50 text-emerald-700"
                    : "text-muted-foreground"
              )}
            >
              {section.title.split(" · ")[1] ?? section.title} {stats.success + stats.error}/{stats.total}
            </a>
          )
        })}
      </nav>

      {QA_SECTIONS.map((section) => {
        const stats = sectionStats[section.key] ?? {
          success: 0,
          error: 0,
          total: section.tests.length,
        }
        return (
          <Card key={section.key} id={section.key} className="scroll-mt-4">
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <CardTitle>{section.title}</CardTitle>
                <span className="text-muted-foreground text-sm">
                  {stats.success + stats.error}/{stats.total} · {stats.success} ✓
                  {stats.error > 0 && <span className="text-destructive"> · {stats.error} ✗</span>}
                </span>
              </div>
              {section.intro && <p className="text-muted-foreground text-sm">{section.intro}</p>}
              <div className="bg-muted/50 rounded-[var(--radius-control)] px-3 py-2 text-sm">
                <p className="font-medium">Bereitstellen:</p>
                <ul className="text-muted-foreground list-disc pl-5">
                  {section.haveReady.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </div>
            </CardHeader>
            <CardContent className="space-y-1">
              {section.tests.map((test) => {
                const state = results[test.key] ?? { status: null, note: "" }
                const isExpanded = expanded[test.key] ?? false
                return (
                  <div
                    key={test.key}
                    className={cn(
                      "border-border rounded-[var(--radius-control)] border",
                      state.status === "error" && "border-destructive/40 bg-destructive/5",
                      state.status === "success" && "border-emerald-600/30 bg-emerald-500/5"
                    )}
                  >
                    <div className="flex items-center gap-3 px-3 py-2">
                      <div className="flex shrink-0 gap-1" role="radiogroup" aria-label={test.title}>
                        {STATUS_OPTIONS.map((option) => (
                          <button
                            key={option.label}
                            type="button"
                            role="radio"
                            aria-checked={state.status === option.value}
                            title={option.title}
                            onClick={() => setStatus(test.key, option.value)}
                            className={cn(
                              "border-border h-7 w-7 rounded-[var(--radius-control)] border text-sm leading-none",
                              state.status === option.value
                                ? option.value === "success"
                                  ? "border-emerald-600 bg-emerald-600 text-white"
                                  : option.value === "error"
                                    ? "border-destructive bg-destructive text-white"
                                    : "border-foreground bg-foreground text-background"
                                : "text-muted-foreground hover:bg-muted"
                            )}
                          >
                            {option.label}
                          </button>
                        ))}
                      </div>
                      <button
                        type="button"
                        onClick={() => setExpanded((map) => ({ ...map, [test.key]: !isExpanded }))}
                        className="flex min-w-0 flex-1 items-center gap-2 text-left"
                      >
                        <span className="min-w-0 flex-1 truncate text-sm font-medium">
                          {test.title}
                          {state.note && !isExpanded && (
                            <span className="text-muted-foreground font-normal"> · Notiz</span>
                          )}
                        </span>
                        {isExpanded ? (
                          <ChevronDown className="text-muted-foreground h-4 w-4 shrink-0" />
                        ) : (
                          <ChevronRight className="text-muted-foreground h-4 w-4 shrink-0" />
                        )}
                      </button>
                    </div>
                    {isExpanded && (
                      <div className="border-border space-y-3 border-t px-3 py-3 text-sm">
                        <div>
                          <p className="text-muted-foreground font-medium">Aktion</p>
                          <p>{test.action}</p>
                        </div>
                        <div>
                          <p className="text-muted-foreground font-medium">Erwartet</p>
                          <p>{test.expected}</p>
                        </div>
                        <div>
                          <p className="text-muted-foreground font-medium">Notiz</p>
                          <Textarea
                            value={noteValue(test.key)}
                            onChange={(e) => setNoteDrafts((map) => ({ ...map, [test.key]: e.target.value }))}
                            onBlur={() => commitNote(test.key)}
                            placeholder="Beobachtungen, Ticket-Referenzen …"
                            rows={2}
                          />
                        </div>
                      </div>
                    )}
                  </div>
                )
              })}
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
