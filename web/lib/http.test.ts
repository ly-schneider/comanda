import { describe, expect, it } from "vitest"
import { readApiError, readErrorMessage } from "@/lib/http"

function jsonRes(body: unknown, status = 400): Response {
  return new Response(JSON.stringify(body), { status })
}

describe("readApiError", () => {
  it("maps a known backend code to German", async () => {
    const info = await readApiError(jsonRes({ code: "slug_taken", message: "slug already exists" }))
    expect(info.messageDe).toBe("Dieser Slug ist bereits vergeben.")
    expect(info.code).toBe("slug_taken")
  })

  it("prefers a sentinel message over the code", async () => {
    const info = await readApiError(jsonRes({ code: "update_failed", message: "jeton_required" }))
    expect(info.messageDe).toBe("Im Jeton-Modus muss ein Jeton zugewiesen sein.")
    expect(info.raw).toBe("jeton_required")
  })

  it("falls back to the caller message for unmapped errors", async () => {
    const info = await readApiError(jsonRes({ code: "something_new", message: "english detail" }), "Kontext-Fallback.")
    expect(info.messageDe).toBe("Kontext-Fallback.")
    expect(info.raw).toBe("english detail")
  })

  it("never returns the backend's raw text without a mapping", async () => {
    const info = await readApiError(jsonRes({ message: "Internal server error" }))
    expect(info.messageDe).toBe("Es ist ein Fehler aufgetreten. Bitte versuche es erneut.")
  })

  it("maps 429 regardless of body", async () => {
    const info = await readApiError(jsonRes({ code: "slug_taken" }, 429))
    expect(info.messageDe).toBe("Zu viele Anfragen. Bitte warte einen Moment und versuche es erneut.")
  })

  it("handles a non-JSON body", async () => {
    const info = await readApiError(new Response("<html>", { status: 502 }), "Kaputt.")
    expect(info).toEqual({ code: null, raw: null, messageDe: "Kaputt." })
  })
})

describe("readErrorMessage", () => {
  it("returns the mapped German message", async () => {
    expect(await readErrorMessage(jsonRes({ code: "not_found" }))).toBe("Der Eintrag wurde nicht gefunden.")
  })
})
