import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { OfflineReadiness } from "@/components/station/offline-readiness"
import type { OfflineReadiness as Readiness } from "@/lib/station/use-offline-readiness"

const useOfflineReadiness = vi.fn<() => Readiness>()
vi.mock("@/lib/station/use-offline-readiness", () => ({
  useOfflineReadiness: () => useOfflineReadiness(),
}))

function mock(state: Readiness["state"], online = true) {
  useOfflineReadiness.mockReturnValue({ state, online, prepare: vi.fn() })
}

afterEach(cleanup)

describe("OfflineReadiness", () => {
  it("renders nothing while the persisted state is still loading", () => {
    mock("checking")
    const { container } = render(<OfflineReadiness />)
    expect(container).toBeEmptyDOMElement()
  })

  it("renders nothing when service workers are unsupported", () => {
    mock("unsupported")
    const { container } = render(<OfflineReadiness />)
    expect(container).toBeEmptyDOMElement()
  })

  it("only shows the offline confirmation in the ready state", () => {
    mock("ready")
    render(<OfflineReadiness />)
    expect(screen.getByText("Bereit für Offline-Betrieb")).toBeInTheDocument()
  })

  it("does not claim readiness before setup completes — it offers the explicit ritual", () => {
    mock("unprepared")
    render(<OfflineReadiness />)
    expect(screen.queryByText(/Bereit für Offline-Betrieb/)).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Für Offline-Betrieb vorbereiten" })).toBeEnabled()
  })

  it("disables the ritual while offline and explains why", () => {
    mock("unprepared", false)
    render(<OfflineReadiness />)
    expect(screen.getByRole("button", { name: "Für Offline-Betrieb vorbereiten" })).toBeDisabled()
    expect(screen.getByText(/Mit dem Internet verbinden/)).toBeInTheDocument()
  })

  it("shows progress while preparing", () => {
    mock("preparing")
    render(<OfflineReadiness />)
    expect(screen.getByText(/Wird für Offline-Betrieb vorbereitet/)).toBeInTheDocument()
  })

  it("prompts a re-run after a service worker upgrade (stale)", () => {
    mock("stale")
    render(<OfflineReadiness />)
    expect(screen.getByText("App wurde aktualisiert")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Erneut vorbereiten" })).toBeInTheDocument()
  })

  it("surfaces a retry on error", () => {
    mock("error")
    render(<OfflineReadiness />)
    expect(screen.getByText("Vorbereitung fehlgeschlagen")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Erneut versuchen" })).toBeInTheDocument()
  })
})
