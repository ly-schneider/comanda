"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"

type Tenant = { id: string; slug: string; name: string; status: string; createdAt: string }

export default function PlatformAdminHome() {
  const fetchAuth = useAuthorizedFetch()
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    ;(async () => {
      try {
        const res = await fetchAuth("/api/v1/platform/tenants")
        if (!res.ok) throw new Error(`Error ${res.status}`)
        const data = (await res.json()) as { items: Tenant[] }
        setTenants(data.items ?? [])
      } catch (e) {
        setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
      } finally {
        setLoading(false)
      }
    })()
  }, [fetchAuth])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="font-primary text-2xl">Tenants</h1>
        <Button asChild>
          <Link href="/platform-admin/tenants/new">Tenant erstellen</Link>
        </Button>
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
          <CardTitle>Alle Tenants</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-muted-foreground text-sm">Laden…</p>
          ) : tenants.length === 0 ? (
            <p className="text-muted-foreground text-sm">Noch keine Tenants.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Aktion</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tenants.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell className="font-medium">{t.name}</TableCell>
                    <TableCell className="text-muted-foreground">{t.slug}</TableCell>
                    <TableCell>{t.status}</TableCell>
                    <TableCell className="text-right">
                      <Button asChild variant="outline" size="sm">
                        <Link href={`/platform-admin/tenants/${t.id}`}>Verwalten</Link>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
