"use client"
import { LogOut, Pencil, UserRound } from "lucide-react"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/contexts/auth-context"
import { authClient } from "@/lib/auth/client"
import { authErrorDe } from "@/lib/auth/localization-de"

// Identity + quick name edit + sign out, shared by the account hub and
// platform-admin chrome so both surfaces show who is signed in.
export function UserMenu() {
  const { signOut, user } = useAuth()
  const fullName = [user?.firstName, user?.lastName].filter(Boolean).join(" ")
  const triggerLabel = fullName || user?.email || "Konto"

  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(fullName)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!editing) setName(fullName)
  }, [editing, fullName])

  async function saveName() {
    setSaving(true)
    setError(null)
    try {
      const result = await authClient.updateUser({ name: name.trim() })
      if (result.error) throw new Error(authErrorDe(result.error, "Name konnte nicht gespeichert werden"))
      setEditing(false)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Name konnte nicht gespeichert werden")
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" className="gap-2">
            <UserRound className="size-4" />
            <span className="max-w-40 truncate">{triggerLabel}</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-56">
          <DropdownMenuLabel className="flex flex-col gap-0.5">
            <span className="truncate font-medium">{fullName || "Kein Name"}</span>
            {user?.email && <span className="text-muted-foreground truncate text-xs font-normal">{user.email}</span>}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setEditing(true)}>
            <Pencil className="size-4" />
            Name bearbeiten
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onSelect={() => void signOut()}>
            <LogOut className="size-4" />
            Abmelden
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={editing} onOpenChange={setEditing}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Name bearbeiten</DialogTitle>
            <DialogDescription>Dieser Name wird in deinem Konto angezeigt.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Dein Name"
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter" && name.trim() && !saving) void saveName()
              }}
            />
            {error && <p className="text-sm text-red-600">{error}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(false)} disabled={saving}>
              Abbrechen
            </Button>
            <Button onClick={() => void saveName()} disabled={saving || !name.trim()}>
              {saving ? "Speichern…" : "Speichern"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
