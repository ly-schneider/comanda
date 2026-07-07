"use client"

import { useCallback, useEffect, useState } from "react"
import { useAccountTenant } from "@/components/account/account-tenant-context"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
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
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<"email", string>>>({})
  const [creating, setCreating] = useState(false)
  const [success, setSuccess] = useState(false)
  const [revokeId, setRevokeId] = useState<string | null>(null)

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
    const trimmed = email.trim()
    if (!trimmed) return
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) {
      setFieldErrors({ email: "Ungültige E-Mail-Adresse." })
      return
    }
    setCreating(true)
    try {
      const res = await fetchAuth(base, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
        body: JSON.stringify({ email: trimmed, memberRole: "admin" }),
      })
      if (!res.ok) {
        setError(await readErrorMessage(res, "Einladung konnte nicht gesendet werden."))
        return
      }
      setEmail("")
      setFieldErrors({})
      setError(null)
      setSuccess(true)
      setTimeout(() => setSuccess(false), 3000)
      await reload()
    } finally {
      setCreating(false)
    }
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
        <h1 className="font-primary text-xl font-semibold">Administrator-Einladungen</h1>
        <p className="text-muted-foreground text-sm">
          Eingeladene Administratoren können Marke, Domains und Zahlungen verwalten.
        </p>
      </div>
      {error && <div className="text-sm text-red-600">{error}</div>}
      {success && (
        <div role="status" className="rounded bg-green-100 px-3 py-2 text-sm text-green-800">
          Einladung gesendet
        </div>
      )}

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
        <Button variant="outline" size="sm" className="h-8" onClick={() => void createInvite()} disabled={creating}>
          Einladen
        </Button>
      </div>

      <TableContainer>
        <div className="overflow-x-auto">
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
                    <Button size="sm" variant="destructive" onClick={() => setRevokeId(i.id)}>
                      Zurückziehen
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </TableContainer>

      <AlertDialog
        open={revokeId !== null}
        onOpenChange={(open) => {
          if (!open) setRevokeId(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Einladung zurückziehen?</AlertDialogTitle>
            <AlertDialogDescription>
              Die Einladung wird widerrufen und kann nicht mehr verwendet werden.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => {
                if (revokeId) void deleteInvite(revokeId)
                setRevokeId(null)
              }}
            >
              Zurückziehen
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
