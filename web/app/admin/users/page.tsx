"use client"
import { useEffect, useState } from "react"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"

type AdminUser = {
  id: string
  name: string
  email: string
  emailVerified: boolean
  isAnonymous?: boolean
  banned?: boolean
  createdAt: string
  updatedAt: string
}

export default function AdminUsersPage() {
  const fetchAuth = useAuthorizedFetch()
  const [items, setItems] = useState<AdminUser[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetchAuth(`/api/v1/users`)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = (await res.json()) as { items: AdminUser[]; count: number }
        if (!cancelled) setItems(data.items || [])
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : "Benutzer konnten nicht geladen werden"
        if (!cancelled) setError(msg)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [fetchAuth])

  return (
    <div className="min-w-0 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Benutzer</h1>
      </div>

      {error && <div className="text-sm text-red-600">{error}</div>}

      <TableContainer>
        <div
          className="overflow-x-auto overscroll-x-contain"
          role="region"
          aria-label="Users table - scroll horizontally to reveal more columns"
          tabIndex={0}
        >
          <Table className="whitespace-nowrap">
            <TableHeader className="bg-card sticky top-0">
              <TableRow>
                <TableHead className="whitespace-nowrap">Name</TableHead>
                <TableHead className="whitespace-nowrap">E-Mail</TableHead>
                <TableHead className="whitespace-nowrap">Verifiziert</TableHead>
                <TableHead className="whitespace-nowrap">Erstellt</TableHead>
                <TableHead className="whitespace-nowrap">Aktualisiert</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((u) => {
                const created = u.createdAt ? new Date(u.createdAt).toLocaleString("de-CH") : "-"
                const updated = u.updatedAt ? new Date(u.updatedAt).toLocaleString("de-CH") : "-"
                return (
                  <TableRow key={u.id} className="even:bg-card odd:bg-muted/40">
                    <TableCell>{u.name || "-"}</TableCell>
                    <TableCell>{u.email}</TableCell>
                    <TableCell>{u.emailVerified ? "Ja" : "Nein"}</TableCell>
                    <TableCell className="whitespace-nowrap">{created}</TableCell>
                    <TableCell className="whitespace-nowrap">{updated}</TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </TableContainer>
    </div>
  )
}
