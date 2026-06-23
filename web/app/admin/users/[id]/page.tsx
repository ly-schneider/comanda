"use client"
import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"

type UserDetails = {
  id: string
  email: string
  name?: string | null
  emailVerified?: boolean
  createdAt?: string
  updatedAt?: string
}

export default function AdminUserDetailPage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  const fetchAuth = useAuthorizedFetch()
  const [user, setUser] = useState<UserDetails | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState<boolean>(false)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      setError(null)
      try {
        const res = await fetchAuth(`/api/v1/users/${encodeURIComponent(id)}`)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = (await res.json()) as UserDetails
        if (!cancelled) {
          setUser(data)
        }
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

  async function deleteUser() {
    if (!confirm("Benutzer wirklich löschen? Diese Aktion kann nicht rückgängig gemacht werden.")) return
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`/api/v1/users/${encodeURIComponent(id)}`, {
        method: "DELETE",
        headers: { "X-CSRF": csrf || "" },
      })
      if (!res.ok && res.status !== 204) throw new Error(`HTTP ${res.status}`)
      router.push("/admin/users")
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Löschen fehlgeschlagen"
      setError(msg)
    }
  }

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
      {error && <div className="text-sm text-red-600">{error}</div>}

      {user && (
        <div className="grid gap-4 md:grid-cols-2">
          <div className="rounded-md border p-4">
            <h2 className="mb-3 text-base font-semibold">Details</h2>
            <div className="space-y-1 text-sm">
              <div>
                <span className="text-muted-foreground">ID:</span> <span className="text-xs">{user.id}</span>
              </div>
              <div>
                <span className="text-muted-foreground">E-Mail:</span> {user.email}
              </div>
              <div>
                <span className="text-muted-foreground">Name:</span> {user.name || "-"}
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

          <div className="rounded-md border p-4 md:col-span-1">
            <h2 className="mb-3 text-base font-semibold">Gefährlicher Bereich</h2>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Benutzer löschen</p>
                <p className="text-muted-foreground text-xs">Entfernt den Benutzer dauerhaft.</p>
              </div>
              <Button variant="destructive" onClick={() => void deleteUser()}>
                Benutzer löschen
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
