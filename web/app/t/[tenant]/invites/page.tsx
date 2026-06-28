"use client"

import { useCallback, useEffect, useState } from "react"
import { useAccountTenant } from "@/components/account/account-tenant-context"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { ROLE_LABELS_DE } from "@/lib/auth/rbac"
import { getCSRFToken } from "@/lib/csrf"
import { readErrorMessage } from "@/lib/http"

type Invite = {
  id: string
  inviteeEmail: string
  memberRole: string
  status: string
  invitedByUserId: string
  inviter?: { id: string; name: string | null; email?: string }
  expiresAt: string
  createdAt: string
}

// The account hub invites administrators — the settings tier that can manage
// branding, domains and payments. App-level roles are invited from the tenant
// /admin surface instead.
export default function AccountInvitesPage() {
  const fetchAuth = useAuthorizedFetch()
  const { activeSlug } = useAccountTenant()
  const base = `/api/v1/account/${activeSlug}/invites`
  const [items, setItems] = useState<Invite[]>([])
  const [email, setEmail] = useState("")
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(async () => {
    try {
      const res = await fetchAuth(base)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = (await res.json()) as { items: Invite[] }
      setItems(data.items || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Einladungen konnten nicht geladen werden")
    }
  }, [fetchAuth, base])

  useEffect(() => {
    void reload()
  }, [reload])

  async function createInvite() {
    if (!email.trim()) return
    const res = await fetchAuth(base, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
      body: JSON.stringify({ email: email.trim(), memberRole: "admin" }),
    })
    if (!res.ok) {
      setError(await readErrorMessage(res))
      return
    }
    setEmail("")
    await reload()
  }

  async function deleteInvite(id: string) {
    try {
      const res = await fetchAuth(`${base}/${encodeURIComponent(id)}`, {
        method: "DELETE",
        headers: { "X-CSRF": getCSRFToken() || "" },
      })
      if (!res.ok && res.status !== 204) throw new Error(await readErrorMessage(res))
      await reload()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Löschen fehlgeschlagen")
    }
  }

  return (
    <div className="min-w-0 space-y-4">
      <div>
        <h1 className="font-primary text-xl">Administrator-Einladungen</h1>
        <p className="text-muted-foreground text-sm">
          Eingeladene Administratoren können Marke, Domains und Zahlungen verwalten.
        </p>
      </div>
      {error && <div className="text-sm text-red-600">{error}</div>}

      <div className="flex items-center gap-2">
        <Input
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="E-Mail-Adresse"
          className="h-8 w-72"
        />
        <Button variant="outline" size="sm" className="h-8" onClick={() => void createInvite()}>
          Einladen
        </Button>
      </div>

      <div className="rounded-md border">
        <div className="max-w-full overflow-x-auto">
          <Table className="whitespace-nowrap">
            <TableHeader className="bg-card sticky top-0">
              <TableRow>
                <TableHead>Eingeladene E-Mail</TableHead>
                <TableHead>Rolle</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Erstellt</TableHead>
                <TableHead>Läuft ab</TableHead>
                <TableHead className="text-right">Aktionen</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((i) => (
                <TableRow key={i.id} className="even:bg-card odd:bg-muted/40">
                  <TableCell>{i.inviteeEmail}</TableCell>
                  <TableCell>{ROLE_LABELS_DE[i.memberRole] ?? i.memberRole}</TableCell>
                  <TableCell className="uppercase">{i.status}</TableCell>
                  <TableCell className="whitespace-nowrap">{new Date(i.createdAt).toLocaleString("de-CH")}</TableCell>
                  <TableCell className="whitespace-nowrap">{new Date(i.expiresAt).toLocaleString("de-CH")}</TableCell>
                  <TableCell className="text-right">
                    <Button size="sm" variant="destructive" onClick={() => void deleteInvite(i.id)}>
                      Löschen
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </div>
    </div>
  )
}
