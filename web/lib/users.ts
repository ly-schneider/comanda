import type { MemberTier } from "@/lib/auth/rbac"

export type AdminUser = {
  id: string
  name: string
  email: string
  emailVerified: boolean
  role?: MemberTier
  createdAt: string
  updatedAt: string
}

const USER_ERROR_DE: Record<string, string> = {
  forbidden_self: "Du kannst deine eigene Rolle nicht ändern oder dich selbst entfernen.",
  forbidden_role_change: "Für diese Person fehlen dir die Berechtigungen.",
  forbidden_higher_role: "Du kannst keine Rolle vergeben, die deiner eigenen entspricht oder darüber liegt.",
  invalid_role: "Ungültige Rolle.",
  not_found: "Benutzer nicht gefunden.",
}

const STATUS_ERROR_DE: Record<number, string> = {
  401: "Nicht angemeldet.",
  403: "Für diese Person fehlen dir die Berechtigungen.",
  404: "Benutzer nicht gefunden.",
}

export async function readUserError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { code?: string }
    const mapped = body.code ? USER_ERROR_DE[body.code] : undefined
    if (mapped) return mapped
  } catch {}
  return STATUS_ERROR_DE[res.status] ?? `Fehler (${res.status})`
}
