import { describe, expect, it } from "vitest"
import { authErrorDe } from "@/lib/auth/localization-de"

describe("authErrorDe", () => {
  it("maps known Better Auth codes to German", () => {
    expect(authErrorDe({ code: "INVALID_OTP", message: "Invalid OTP" }, "Fallback.")).toBe(
      "Ungültiger Code. Bitte überprüfe deine Eingabe."
    )
    expect(authErrorDe({ code: "OTP_EXPIRED" }, "Fallback.")).toBe(
      "Der Code ist abgelaufen. Bitte fordere einen neuen an."
    )
  })

  it("maps 429 before codes", () => {
    expect(authErrorDe({ code: "INVALID_OTP", status: 429 }, "Fallback.")).toBe(
      "Zu viele Anfragen. Bitte warte einen Moment und versuche es erneut."
    )
  })

  it("never returns the English message for unknown codes", () => {
    expect(authErrorDe({ code: "SOMETHING_NEW", message: "English text" }, "Fallback.")).toBe("Fallback.")
    expect(authErrorDe(null, "Fallback.")).toBe("Fallback.")
  })
})
