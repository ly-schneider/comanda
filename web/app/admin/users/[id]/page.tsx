"use client"
import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { useCallback, useEffect, useState } from "react"
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
import { useAuth } from "@/contexts/auth-context"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { useMemberTier } from "@/hooks/use-member-tier"
import { ROLE_LABELS_DE, TIER_RANK } from "@/lib/auth/rbac"
import { getCSRFToken } from "@/lib/csrf"
import { type AdminUser, readUserError } from "@/lib/users"

export default function AdminUserDetailPage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  const fetchAuth = useAuthorizedFetch()
  const { user: actor } = useAuth()
  const { tier } = useMemberTier()
  const [user, setUser] = useState<AdminUser | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState<boolean>(false)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      setError(null)
      try {
        const res = await fetchAuth(`/api/v1/users/${encodeURIComponent(id)}`)
        if (!res.ok) throw new Error(await readUserError(res))
        const data = (await res.json()) as AdminUser
        if (!cancelled) setUser(data)
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : "Fehler beim Laden des Benutzers"
        if (!cancelled) setError(msg)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [fetchAuth, id])

  const created = user?.createdAt ? new Date(user.createdAt).toLocaleString("de-CH") : "-"
  const updated = user?.updatedAt ? new Date(user.updatedAt).toLocaleString("de-CH") : "-"

  // Mirror the backend guard: members strictly below your own tier, never
  // yourself. No role = a customer, so there is no membership to remove.
  const actorRank = tier ? (TIER_RANK[tier] ?? 0) : 0
  const removable = !!user?.role && (TIER_RANK[user.role] ?? 0) < actorRank && user.id !== actor?.id

  const removeMember = useCallback(async () => {
    setError(null)
    try {
      const res = await fetchAuth(`/api/v1/users/${encodeURIComponent(id)}`, {
        method: "DELETE",
        headers: { "X-CSRF": getCSRFToken() || "" },
      })
      if (!res.ok && res.status !== 204) throw new Error(await readUserError(res))
      router.push("/admin/users")
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Entfernen fehlgeschlagen")
    }
  }, [fetchAuth, id, router])

  return (
    <div className="min-w-0 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Benutzer</h1>
        <div className="flex items-center gap-2">
          <Link href="/admin/users">
            <Button variant="outline">Zurück zur Übersicht</Button>
          </Link>
        </div>
      </div>

      {loading && <div className="text-muted-foreground text-sm">Lade Benutzer...</div>}
      {error && <div className="text-destructive text-sm">{error}</div>}

      {user && (
        <div className="grid gap-4 md:grid-cols-2">
          <div className="rounded-md border p-4">
            <h2 className="mb-3 text-base font-semibold">Details</h2>
            <div className="space-y-1 text-sm">
              <div>
                <span className="text-muted-foreground">ID:</span> <span className="text-xs">{user.id}</span>
              </div>
              <div>
                <span className="text-muted-foreground">E-Mail:</span> {user.email || "-"}
              </div>
              <div>
                <span className="text-muted-foreground">Name:</span> {user.name || "-"}
              </div>
              <div>
                <span className="text-muted-foreground">Rolle:</span>{" "}
                {user.role ? ROLE_LABELS_DE[user.role] : "Kunde (kein Teammitglied)"}
              </div>
              <div>
                <span className="text-muted-foreground">Verifiziert:</span> {user.emailVerified ? "Ja" : "Nein"}
              </div>
              <div>
                <span className="text-muted-foreground">Erstellt:</span> {created}
              </div>
              <div>
                <span className="text-muted-foreground">Aktualisiert:</span> {updated}
              </div>
            </div>
          </div>

          {removable && (
            <div className="rounded-md border p-4">
              <h2 className="mb-3 text-base font-semibold">Gefährlicher Bereich</h2>
              <div className="flex items-center justify-between gap-4">
                <div>
                  <p className="text-sm font-medium">Aus dem Team entfernen</p>
                  <p className="text-muted-foreground text-xs">
                    Das Benutzerkonto bleibt bestehen und kann erneut eingeladen werden.
                  </p>
                </div>
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button variant="destructive">Entfernen</Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Aus dem Team entfernen?</AlertDialogTitle>
                      <AlertDialogDescription>
                        {user.name || user.email} verliert den Zugriff auf dieses Tenant. Das Benutzerkonto selbst
                        bleibt bestehen und kann erneut eingeladen werden.
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>Abbrechen</AlertDialogCancel>
                      <AlertDialogAction
                        className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        onClick={() => void removeMember()}
                      >
                        Entfernen
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
