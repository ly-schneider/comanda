/**
 * German localization strings for authentication UI.
 *
 * These strings are used in custom auth components (login, invite accept, etc.)
 * for consistent German language support throughout the auth flow.
 */
export const authLocalizationDe = {
  // Email
  email: "E-Mail",
  emailPlaceholder: "m@beispiel.de",
  emailRequired: "E-Mail-Adresse ist erforderlich",

  // Email OTP
  emailOtp: "E-Mail-Code",
  emailOtpSendAction: "Code senden",
  emailOtpVerifyAction: "Code bestätigen",
  emailOtpDescription: "Gib deine E-Mail-Adresse ein, um einen Code zu erhalten",
  emailOtpVerificationSent: "Bitte überprüfe deine E-Mails für den Bestätigungscode.",

  // Sign in/out
  signIn: "Anmelden",
  signInWith: "Anmelden mit",
  signOut: "Abmelden",

  // Social/Providers
  orContinueWith: "Oder weiter mit",

  // Actions
  continue: "Weiter",
  goBack: "Zurück",
  resendCode: "Code erneut senden",

  // Errors
  isInvalid: "ist ungültig",
  isRequired: "ist erforderlich",
  requestFailed: "Anfrage fehlgeschlagen",
} as const

export type AuthLocalizationDe = typeof authLocalizationDe

const authErrorCodesDe: Record<string, string> = {
  INVALID_OTP: "Ungültiger Code. Bitte überprüfe deine Eingabe.",
  OTP_EXPIRED: "Der Code ist abgelaufen. Bitte fordere einen neuen an.",
  TOO_MANY_ATTEMPTS: "Zu viele Fehlversuche. Bitte fordere einen neuen Code an.",
  SESSION_EXPIRED: "Bitte melde dich erneut an.",
}

type AuthError = { code?: string | null; message?: string | null; status?: number } | null | undefined

// Better Auth error messages are English; show mapped German copy and never
// leak the raw message.
export function authErrorDe(error: AuthError, fallback: string): string {
  if (error?.status === 429) return "Zu viele Anfragen. Bitte warte einen Moment und versuche es erneut."
  const mapped = error?.code ? authErrorCodesDe[error.code] : undefined
  return mapped ?? fallback
}
