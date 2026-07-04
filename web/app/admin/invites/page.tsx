"use client"
import Link from "next/link"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { ROLE_LABELS_DE } from "@/lib/auth/rbac"
import { getCSRFToken } from "@/lib/csrf"
import { readErrorMessage } from "@/lib/http"

type InviteMemberRole = "app_admin" | "staff" | "catalog"

type Invite = {
  id: string
  inviteeEmail: string
  memberRole: InviteMemberRole
  status: string
  invitedByUserId: string
  inviter?: { id: string; name: string | null; email?: string }
  expiresAt: string
  createdAt: string
}

export default function AdminInvitesPage() {
  const fetchAuth = useAuthorizedFetch()
  const [items, setItems] = useState<Invite[]>([])
  const [email, setEmail] = useState("")
  const [memberRole, setMemberRole] = useState<InviteMemberRole>("staff")
  const [error, setError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<"email", string>>>({})
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    void reload()
  }, [])

  async function reload() {
    try {
      const res = await fetchAuth(`/api/v1/invites`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = (await res.json()) as { items: Invite[] }
      setItems(data.items || [])
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to load invites"
      setError(msg)
    }
  }

  async function createInvite() {
    const trimmed = email.trim()
    if (!trimmed) return
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) {
      setFieldErrors({ email: "Ungültige E-Mail-Adresse." })
      return
    }
    setCreating(true)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`/api/v1/invites`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({ email: trimmed, memberRole }),
      })
      if (!res.ok) {
        setError(await readErrorMessage(res, "Einladung konnte nicht gesendet werden."))
        return
      }
      setEmail("")
      setMemberRole("staff")
      setFieldErrors({})
      await reload()
    } finally {
      setCreating(false)
    }
  }

  async function deleteInvite(id: string) {
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`/api/v1/invites/${encodeURIComponent(id)}`, {
        method: "DELETE",
        headers: { "X-CSRF": csrf || "" },
      })
      if (!res.ok && res.status !== 204) throw new Error(await readErrorMessage(res))
      await reload()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Löschen fehlgeschlagen"
      setError(msg)
    }
  }

  return (
    <div className="min-w-0 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Admin-Einladungen</h1>
      </div>
      {error && <div className="text-sm text-red-600">{error}</div>}

      <div className="flex items-start gap-2">
        <div>
          <Input
            type="email"
            value={email}
            onChange={(e) => {
              setEmail(e.target.value)
              if (fieldErrors.email) setFieldErrors({})
            }}
            placeholder="E-Mail-Adresse"
            className="h-8 w-72"
            aria-invalid={!!fieldErrors.email}
          />
          {fieldErrors.email && <p className="text-destructive mt-1 text-sm">{fieldErrors.email}</p>}
        </div>
        <Select value={memberRole} onValueChange={(v) => setMemberRole(v as InviteMemberRole)}>
          <SelectTrigger className="h-8 w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="app_admin">{ROLE_LABELS_DE.app_admin}</SelectItem>
            <SelectItem value="staff">{ROLE_LABELS_DE.staff}</SelectItem>
            <SelectItem value="catalog">{ROLE_LABELS_DE.catalog}</SelectItem>
          </SelectContent>
        </Select>
        <Button variant="outline" size="sm" className="h-8" onClick={() => void createInvite()} disabled={creating}>
          Einladen
        </Button>
      </div>

      <TableContainer>
        <div
          className="overflow-x-auto overscroll-x-contain"
          role="region"
          aria-label="Invites table – scroll horizontally to reveal more columns"
          tabIndex={0}
        >
          <Table className="whitespace-nowrap">
            <TableHeader className="bg-card sticky top-0">
              <TableRow>
                <TableHead className="whitespace-nowrap">ID</TableHead>
                <TableHead className="whitespace-nowrap">Eingeladene E-Mail</TableHead>
                <TableHead className="whitespace-nowrap">Rolle</TableHead>
                <TableHead className="whitespace-nowrap">Status</TableHead>
                <TableHead className="whitespace-nowrap">Eingeladen von</TableHead>
                <TableHead className="whitespace-nowrap">Erstellt</TableHead>
                <TableHead className="whitespace-nowrap">Läuft ab</TableHead>
                <TableHead className="text-right whitespace-nowrap">Aktionen</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((i) => {
                const created = new Date(i.createdAt).toLocaleString("de-CH")
                const expires = new Date(i.expiresAt).toLocaleString("de-CH")
                const inviterId = i.inviter?.id ?? i.invitedByUserId
                const inviter = (
                  <Link
                    href={`/admin/users/${encodeURIComponent(inviterId)}`}
                    className="text-xs underline underline-offset-2"
                  >
                    {i.inviter?.name || i.invitedByUserId}
                  </Link>
                )
                return (
                  <TableRow key={i.id} className="even:bg-card odd:bg-muted/40">
                    <TableCell className="text-xs">{i.id}</TableCell>
                    <TableCell>{i.inviteeEmail}</TableCell>
                    <TableCell>{ROLE_LABELS_DE[i.memberRole] ?? i.memberRole}</TableCell>
                    <TableCell className="uppercase">{i.status}</TableCell>
                    <TableCell>{inviter}</TableCell>
                    <TableCell className="whitespace-nowrap">{created}</TableCell>
                    <TableCell className="whitespace-nowrap">{expires}</TableCell>
                    <TableCell className="text-right">
                      <Button size="sm" variant="destructive" onClick={() => void deleteInvite(i.id)}>
                        Löschen
                      </Button>
                    </TableCell>
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
