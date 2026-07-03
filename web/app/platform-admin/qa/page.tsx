"use client"

import Link from "next/link"
import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"
import { QA_TEST_COUNT } from "@/lib/qa-tests"

type QaRun = {
  id: string
  name: string
  note?: string
  successCount: number
  errorCount: number
  createdAt: string
}

function defaultRunName() {
  return `QA ${new Date().toISOString().slice(0, 10)}`
}

export default function QaRunsPage() {
  const fetchAuth = useAuthorizedFetch()
  const [runs, setRuns] = useState<QaRun[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [name, setName] = useState(defaultRunName)
  const [creating, setCreating] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const res = await fetchAuth("/api/v1/platform/qa/runs")
      if (!res.ok) throw new Error(`Error ${res.status}`)
      const data = (await res.json()) as { items: QaRun[] }
      setRuns(data.items ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
    } finally {
      setLoading(false)
    }
  }, [fetchAuth])

  useEffect(() => {
    load()
  }, [load])

  async function createRun(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setCreating(true)
    setError(null)
    try {
      const res = await fetchAuth("/api/v1/platform/qa/runs", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
        body: JSON.stringify({ name: name.trim() }),
      })
      if (!res.ok) throw new Error(`Error ${res.status}`)
      const created = (await res.json()) as QaRun
      setRuns((prev) => [created, ...prev])
      setName(defaultRunName())
    } catch (e) {
      setError(e instanceof Error ? e.message : "Erstellen fehlgeschlagen")
    } finally {
      setCreating(false)
    }
  }

  async function deleteRun(id: string) {
    setError(null)
    try {
      const res = await fetchAuth(`/api/v1/platform/qa/runs/${id}`, {
        method: "DELETE",
        headers: { "X-CSRF": getCSRFToken() || "" },
      })
      if (!res.ok && res.status !== 204) throw new Error(`Error ${res.status}`)
      setRuns((prev) => prev.filter((r) => r.id !== id))
    } catch (e) {
      setError(e instanceof Error ? e.message : "Löschen fehlgeschlagen")
    } finally {
      setConfirmDelete(null)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="font-primary text-2xl">QA-Checkliste</h1>
      </div>

      {error && (
        <div
          role="alert"
          className="text-destructive bg-destructive/10 rounded-[var(--radius-control)] px-3 py-2 text-sm"
        >
          {error}
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Neuer Testlauf</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={createRun} className="flex gap-2">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="z. B. Staging Pre-Launch"
              maxLength={120}
              className="max-w-sm"
            />
            <Button type="submit" disabled={creating || !name.trim()}>
              {creating ? "Erstellen…" : "Testlauf starten"}
            </Button>
          </form>
          <p className="text-muted-foreground mt-2 text-sm">
            Ein Testlauf umfasst {QA_TEST_COUNT} Tests. Frühere Läufe bleiben als Verlauf erhalten.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Testläufe</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-muted-foreground text-sm">Laden…</p>
          ) : runs.length === 0 ? (
            <p className="text-muted-foreground text-sm">Noch keine Testläufe.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Erstellt</TableHead>
                  <TableHead>Fortschritt</TableHead>
                  <TableHead className="text-right">Aktion</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((run) => {
                  const done = run.successCount + run.errorCount
                  return (
                    <TableRow key={run.id}>
                      <TableCell className="font-medium">
                        <Link href={`/platform-admin/qa/${run.id}`} className="hover:underline">
                          {run.name}
                        </Link>
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {new Date(run.createdAt).toLocaleDateString("de-CH", {
                          dateStyle: "medium",
                        })}
                      </TableCell>
                      <TableCell>
                        <span className="text-sm">
                          {done}/{QA_TEST_COUNT}
                          <span className="text-muted-foreground">
                            {" "}
                            · {run.successCount} ✓
                            {run.errorCount > 0 && <span className="text-destructive"> · {run.errorCount} ✗</span>}
                          </span>
                        </span>
                      </TableCell>
                      <TableCell className="text-right">
                        {confirmDelete === run.id ? (
                          <span className="inline-flex gap-2">
                            <Button variant="destructive" size="sm" onClick={() => deleteRun(run.id)}>
                              Ja, löschen
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => setConfirmDelete(null)}>
                              Abbrechen
                            </Button>
                          </span>
                        ) : (
                          <span className="inline-flex gap-2">
                            <Button asChild variant="outline" size="sm">
                              <Link href={`/platform-admin/qa/${run.id}`}>Öffnen</Link>
                            </Button>
                            <Button variant="ghost" size="sm" onClick={() => setConfirmDelete(run.id)}>
                              Löschen
                            </Button>
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
