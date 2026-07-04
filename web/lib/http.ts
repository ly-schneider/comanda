const GENERIC_DE = "Es ist ein Fehler aufgetreten. Bitte versuche es erneut."

const ERROR_MESSAGES_DE: Record<string, string> = {
  invalid_body: "Ungültige Eingabe. Bitte prüfe die Felder.",
  validation_error: "Ungültige Eingabe. Bitte prüfe die Felder.",
  invalid_json: "Ungültige Eingabe. Bitte prüfe die Felder.",
  invalid_payload: "Ungültige Eingabe. Bitte prüfe die Felder.",
  invalid_id: "Ungültige ID.",
  not_found: "Der Eintrag wurde nicht gefunden.",
  conflict: "Der Eintrag existiert bereits oder steht in Konflikt mit einem anderen.",
  unauthorized: "Keine Berechtigung für diese Aktion.",
  unauthenticated: "Bitte melde dich erneut an.",
  internal_error: GENERIC_DE,
  update_failed: "Speichern fehlgeschlagen. Bitte versuche es erneut.",
  upload_failed: "Upload fehlgeschlagen. Bitte versuche es erneut.",
  invalid_file_type: "Nur JPEG-, PNG- und WebP-Bilder sind erlaubt.",
  invalid_token: "Dieser Link ist ungültig oder abgelaufen.",
  invalid_member_role: "Ungültige Rolle.",
  invite_failed: "Einladung konnte nicht erstellt werden.",
  owner_exists: "Es existiert bereits ein Inhaber für diesen Tenant.",
  slug_taken: "Dieser Slug ist bereits vergeben.",
  invalid_status: "Ungültiger Status.",
  tenant_suspended: "Dieser Tenant ist gesperrt.",
  payment_failed: "Die Zahlung konnte nicht verarbeitet werden.",
  refund_failed: "Erstattung nicht möglich. Bitte prüfe Betrag und Positionen.",
  provider_error: "Der Zahlungsanbieter konnte die Anfrage nicht verarbeiten.",
  configure_failed: "Zugangsdaten konnten nicht überprüft werden.",
  test_failed: "Verbindungstest fehlgeschlagen.",
  jeton_required: "Im Jeton-Modus muss ein Jeton zugewiesen sein.",
  station_no_matching_products: "Der Station sind keine passenden Produkte zugewiesen.",
  no_lines: "Keine Positionen ausgewählt.",
  in_progress: "Der Vorgang läuft bereits.",
}

export type ApiErrorInfo = {
  code: string | null
  // Sentinel service message (e.g. "jeton_required").
  raw: string | null
  messageDe: string
}

export async function readApiError(res: Response, fallback = GENERIC_DE): Promise<ApiErrorInfo> {
  let code: string | null = null
  let raw: string | null = null
  try {
    const j = (await res.json()) as { code?: unknown; message?: unknown; detail?: unknown }
    if (typeof j.code === "string") code = j.code
    if (typeof j.message === "string") raw = j.message
    else if (typeof j.detail === "string") raw = j.detail
  } catch {}

  let messageDe: string | undefined
  if (res.status === 429) {
    messageDe = "Zu viele Anfragen. Bitte warte einen Moment und versuche es erneut."
  } else {
    messageDe = (raw ? ERROR_MESSAGES_DE[raw] : undefined) ?? (code ? ERROR_MESSAGES_DE[code] : undefined)
  }
  return { code, raw, messageDe: messageDe ?? fallback }
}

export async function readErrorMessage(res: Response, fallback = GENERIC_DE): Promise<string> {
  return (await readApiError(res, fallback)).messageDe
}
