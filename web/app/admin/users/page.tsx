"use client"
import { useEffect, useMemo, useState } from "react"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuth } from "@/contexts/auth-context"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { useMemberTier } from "@/hooks/use-member-tier"
import { type MemberTier, ROLE_LABELS_DE, TIER_RANK } from "@/lib/auth/rbac"
import { getCSRFToken } from "@/lib/csrf"
import { type AdminUser, readUserError } from "@/lib/users"

const ALL_ROLES: MemberTier[] = ["owner", "admin", "app_admin", "staff", "catalog"]

export default function AdminUsersPage() {
  const fetchAuth = useAuthorizedFetch()
  const { user } = useAuth()
  const { tier } = useMemberTier()
  const [items, setItems] = useState<AdminUser[]>([])
  const [error, setError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  const actorRank = tier ? (TIER_RANK[tier] ?? 0) : 0
  const assignableRoles = useMemo(() => ALL_ROLES.filter((r) => (TIER_RANK[r] ?? 0) < actorRank), [actorRank])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetchAuth(`/api/v1/users`)
        if (!res.ok) throw new Error(await readUserError(res))
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

  // Mirror the backend guard: you can only manage members strictly below your own
  // tier, and never yourself.
  function canManage(u: AdminUser): boolean {
    return !!u.role && (TIER_RANK[u.role] ?? 0) < actorRank && u.id !== user?.id
  }

  async function changeRole(u: AdminUser, role: MemberTier) {
    setBusyId(u.id)
    setError(null)
    try {
      const res = await fetchAuth(`/api/v1/users/${encodeURIComponent(u.id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", "X-CSRF": getCSRFToken() || "" },
        body: JSON.stringify({ role }),
      })
      if (!res.ok) throw new Error(await readUserError(res))
      setItems((prev) => prev.map((it) => (it.id === u.id ? { ...it, role } : it)))
    } catch (e) {
      setError(e instanceof Error ? e.message : "Rolle ändern fehlgeschlagen")
    } finally {
      setBusyId(null)
    }
  }

  async function removeUser(u: AdminUser) {
    setBusyId(u.id)
    setError(null)
    try {
      const res = await fetchAuth(`/api/v1/users/${encodeURIComponent(u.id)}`, {
        method: "DELETE",
        headers: { "X-CSRF": getCSRFToken() || "" },
      })
      if (!res.ok) throw new Error(await readUserError(res))
      setItems((prev) => prev.filter((it) => it.id !== u.id))
    } catch (e) {
      setError(e instanceof Error ? e.message : "Entfernen fehlgeschlagen")
    } finally {
      setBusyId(null)
    }
  }

  return (
    <div className="min-w-0 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Benutzer</h1>
      </div>

      {error && <div className="text-destructive text-sm">{error}</div>}

      <TableContainer>
        <div
          className="overflow-x-auto overscroll-x-contain"
          role="region"
          aria-label="Benutzertabelle – horizontal scrollen für weitere Spalten"
          tabIndex={0}
        >
          <Table className="whitespace-nowrap">
            <TableHeader className="bg-card sticky top-0">
              <TableRow>
                <TableHead className="whitespace-nowrap">Name</TableHead>
                <TableHead className="whitespace-nowrap">E-Mail</TableHead>
                <TableHead className="whitespace-nowrap">Rolle</TableHead>
                <TableHead className="whitespace-nowrap">Verifiziert</TableHead>
                <TableHead className="whitespace-nowrap">Erstellt</TableHead>
                <TableHead className="text-right whitespace-nowrap">Aktionen</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((u) => {
                const created = u.createdAt ? new Date(u.createdAt).toLocaleString("de-CH") : "-"
                const manageable = canManage(u)
                const busy = busyId === u.id
                return (
                  <TableRow key={u.id} className="even:bg-card odd:bg-muted/40">
                    <TableCell>{u.name || "-"}</TableCell>
                    <TableCell>{u.email}</TableCell>
                    <TableCell>
                      {manageable ? (
                        <Select value={u.role} onValueChange={(v) => changeRole(u, v as MemberTier)} disabled={busy}>
                          <SelectTrigger className="h-9 w-40" aria-label={`Rolle von ${u.name || u.email}`}>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {assignableRoles.map((r) => (
                              <SelectItem key={r} value={r}>
                                {ROLE_LABELS_DE[r]}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <span className="text-muted-foreground">{u.role ? ROLE_LABELS_DE[u.role] : "—"}</span>
                      )}
                    </TableCell>
                    <TableCell>{u.emailVerified ? "Ja" : "Nein"}</TableCell>
                    <TableCell className="whitespace-nowrap">{created}</TableCell>
                    <TableCell className="text-right">
                      {manageable ? (
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button variant="outline" size="sm" disabled={busy}>
                              Entfernen
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>Aus dem Team entfernen?</AlertDialogTitle>
                              <AlertDialogDescription>
                                {u.name || u.email} verliert den Zugriff auf dieses Tenant. Das Benutzerkonto selbst
                                bleibt bestehen und kann erneut eingeladen werden.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>Abbrechen</AlertDialogCancel>
                              <AlertDialogAction
                                className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                                onClick={() => removeUser(u)}
                              >
                                Entfernen
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
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
