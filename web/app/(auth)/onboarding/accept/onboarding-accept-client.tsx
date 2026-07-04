"use client"

import { useRouter, useSearchParams } from "next/navigation"
import { useEffect, useRef, useState } from "react"
import AuthHeader from "@/components/layout/auth-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { InputOTP, InputOTPGroup, InputOTPSlot } from "@/components/ui/input-otp"
import { Label } from "@/components/ui/label"
import { authClient, emailOtp, signIn } from "@/lib/auth/client"
import { authErrorDe } from "@/lib/auth/localization-de"
import { getCSRFToken } from "@/lib/csrf"
import { postAuthPath } from "@/lib/post-auth"

type InviteStep = "verifying" | "send-otp" | "enter-otp" | "accepting" | "error"

type VerifyResponse = { email: string; status: string; expiresAt: string }
type ApiErrorBody = { code?: string; message?: string }

const INVITE_ERROR_MESSAGES: Record<string, string> = {
  invalid_token: "Dieser Einladungslink ist ungültig.",
  email_mismatch: "Die E-Mail-Adresse stimmt nicht mit der Einladung überein.",
  invite_expired: "Diese Einladung ist abgelaufen.",
  invite_used: "Diese Einladung wurde bereits verwendet.",
  invite_not_pending: "Diese Einladung ist nicht mehr aktiv.",
  owner_exists: "Für diesen Tenant existiert bereits ein Inhaber.",
}

export default function OnboardingAcceptClient() {
  const sp = useSearchParams()
  const token = sp.get("token") || ""
  const router = useRouter()

  const { data: session, refetch: refetchSession } = authClient.useSession()
  const [step, setStep] = useState<InviteStep>("verifying")
  const [email, setEmail] = useState("")
  const [otp, setOtp] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Prevents the accept call from firing more than once when the session appears.
  const acceptedRef = useRef(false)

  useEffect(() => {
    let cancelled = false
    async function verify() {
      if (!token) {
        setError(INVITE_ERROR_MESSAGES.invalid_token!)
        setStep("error")
        return
      }
      try {
        const res = await fetch("/api/v1/owner-invites/verify", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ token }),
        })
        const body = (await res.json()) as VerifyResponse | ApiErrorBody
        if (!cancelled) {
          if (!res.ok) {
            const code = (body as ApiErrorBody).code || "invalid_token"
            setError(INVITE_ERROR_MESSAGES[code] ?? "Einladung konnte nicht verifiziert werden.")
            setStep("error")
            return
          }
          setEmail((body as VerifyResponse).email)
          setStep("send-otp")
        }
      } catch {
        if (!cancelled) {
          setError("Verifizierung fehlgeschlagen. Bitte später erneut versuchen.")
          setStep("error")
        }
      }
    }
    verify()
    return () => {
      cancelled = true
    }
  }, [token])

  // After OTP sign-in succeeds the session becomes non-null. Accept the invite,
  // then refresh the session so activeOrganizationId is populated before routing.
  useEffect(() => {
    if (!session?.user || acceptedRef.current) return
    if (step !== "enter-otp") return
    acceptedRef.current = true

    async function accept() {
      setStep("accepting")
      setLoading(true)
      setError(null)
      try {
        const csrf = getCSRFToken() || ""
        const sessionToken = session!.session.token
        const res = await fetch("/api/v1/owner-invites/accept", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${sessionToken}`,
            "X-CSRF": csrf,
          },
          credentials: "include",
          body: JSON.stringify({ token }),
        })
        const body = (await res.json().catch(() => ({}))) as { organizationId?: string } & ApiErrorBody
        if (!res.ok) {
          const code = body.code || ""
          setError(INVITE_ERROR_MESSAGES[code] ?? "Einladung annehmen fehlgeschlagen.")
          setStep("error")
          setLoading(false)
          return
        }
        // Force the React client to re-fetch the session from the server so
        // activeOrganizationId (just written by backend accept) is reflected.
        await refetchSession({ query: { disableCookieCache: true } })
        // Read the freshened session to decide where to land.
        const fresh = await authClient.getSession()
        const activeOrgId = fresh.data?.session.activeOrganizationId
        router.replace(postAuthPath({ isPlatformAdmin: false, activeOrganizationId: activeOrgId }, "/account"))
      } catch {
        setError("Etwas ist schiefgelaufen. Bitte erneut versuchen.")
        setStep("error")
        setLoading(false)
      }
    }
    accept()
  }, [session, step, token, router, refetchSession])

  const handleSendOtp = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await emailOtp.sendVerificationOtp({ email, type: "sign-in" })
      if (result.error) {
        setError(authErrorDe(result.error, "Fehler beim Senden des Codes."))
        setLoading(false)
        return
      }
      setStep("enter-otp")
    } catch {
      setError("Fehler beim Senden des Codes. Bitte erneut versuchen.")
    } finally {
      setLoading(false)
    }
  }

  // Send the code as soon as the invite is verified; the manual button stays
  // as the retry path when sending fails.
  const otpAutoSentRef = useRef(false)
  useEffect(() => {
    if (step !== "send-otp" || otpAutoSentRef.current) return
    otpAutoSentRef.current = true
    void handleSendOtp()
  }, [step])

  const handleOtpSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!otp || otp.length < 6) return
    setLoading(true)
    setError(null)
    try {
      const result = await signIn.emailOtp({ email, otp })
      if (result.error) {
        setError(authErrorDe(result.error, "Ungültiger Code. Bitte erneut versuchen."))
        setLoading(false)
      }
      // On success the session useEffect fires and calls accept.
    } catch {
      setError("Anmeldung fehlgeschlagen. Bitte erneut versuchen.")
      setLoading(false)
    }
  }

  const handleResendOtp = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await emailOtp.sendVerificationOtp({ email, type: "sign-in" })
      if (result.error) {
        setError(authErrorDe(result.error, "Fehler beim Senden des Codes."))
      }
    } catch {
      setError("Fehler beim Senden des Codes.")
    } finally {
      setLoading(false)
    }
  }

  if (step === "verifying" || step === "accepting") {
    return (
      <div className="flex flex-1 flex-col">
        <main className="container mx-auto px-4 pt-24 pb-10">
          <AuthHeader />
          <p className="text-muted-foreground mt-12 text-center text-sm">
            {step === "verifying" ? "Prüfe Einladung…" : "Einladung wird angenommen…"}
          </p>
        </main>
      </div>
    )
  }

  if (step === "error") {
    return (
      <div className="flex flex-1 flex-col">
        <main className="container mx-auto px-4 pt-24 pb-10">
          <AuthHeader />
          <div className="mx-auto mt-12 max-w-sm">
            <div
              role="alert"
              className="bg-destructive/10 text-destructive rounded-[var(--radius-control)] px-4 py-3 text-sm"
            >
              {error ?? "Ungültiger Einladungslink."}
            </div>
          </div>
        </main>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col">
      <main className="container mx-auto px-4 pt-24 pb-10">
        <AuthHeader />
        <h1 className="mt-4 mb-2 text-center text-2xl font-semibold">Als Inhaber registrieren</h1>

        <div className="mx-auto mt-12 max-w-sm space-y-6">
          <div className="space-y-2">
            <Label htmlFor="email">E-Mail-Adresse</Label>
            <Input id="email" type="email" value={email} disabled className="bg-muted" />
          </div>

          {step === "send-otp" && (
            <Button className="h-11 w-full rounded-full" onClick={handleSendOtp} disabled={loading}>
              {loading ? "Sende…" : "Bestätigungscode senden"}
            </Button>
          )}

          {step === "enter-otp" && (
            <form onSubmit={handleOtpSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="otp">Bestätigungscode</Label>
                <p className="text-muted-foreground text-sm">Wir haben einen Code an {email} gesendet.</p>
                <InputOTP maxLength={6} value={otp} onChange={setOtp} disabled={loading} autoFocus>
                  <InputOTPGroup>
                    <InputOTPSlot index={0} />
                    <InputOTPSlot index={1} />
                    <InputOTPSlot index={2} />
                    <InputOTPSlot index={3} />
                    <InputOTPSlot index={4} />
                    <InputOTPSlot index={5} />
                  </InputOTPGroup>
                </InputOTP>
              </div>
              <Button type="submit" className="h-11 w-full rounded-full" disabled={loading || otp.length < 6}>
                {loading ? "Prüfe…" : "Anmelden & Einladung annehmen"}
              </Button>
              <div className="flex justify-end text-sm">
                <button
                  type="button"
                  className="text-muted-foreground hover:text-foreground underline"
                  onClick={handleResendOtp}
                  disabled={loading}
                >
                  Code erneut senden
                </button>
              </div>
            </form>
          )}

          {error && <p className="text-center text-sm text-red-600">{error}</p>}
        </div>
      </main>
    </div>
  )
}
