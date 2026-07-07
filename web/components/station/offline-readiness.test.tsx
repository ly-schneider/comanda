import { cleanup, fireEvent, render, screen } from "@testing-library/react"
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

function openStatus() {
  fireEvent.click(screen.getByRole("button", { name: "Offline-Status" }))
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

  it("keeps the healthy status collapsed behind the control until opened", async () => {
    mock("ready")
    render(<OfflineReadiness />)
    expect(screen.queryByText("Bereit für Offline-Betrieb")).not.toBeInTheDocument()
    openStatus()
    expect(await screen.findByText("Bereit für Offline-Betrieb")).toBeInTheDocument()
  })

  it("does not claim readiness before setup completes — it offers the explicit ritual", async () => {
    mock("unprepared")
    render(<OfflineReadiness />)
    openStatus()
    expect(await screen.findByRole("button", { name: "Für Offline-Betrieb vorbereiten" })).toBeEnabled()
    expect(screen.queryByText(/Bereit für Offline-Betrieb/)).not.toBeInTheDocument()
  })

  it("disables the ritual while offline and explains why", async () => {
    mock("unprepared", false)
    render(<OfflineReadiness />)
    openStatus()
    expect(await screen.findByRole("button", { name: "Für Offline-Betrieb vorbereiten" })).toBeDisabled()
    expect(screen.getByText(/Mit dem Internet verbinden/)).toBeInTheDocument()
  })

  it("shows progress while preparing", async () => {
    mock("preparing")
    render(<OfflineReadiness />)
    openStatus()
    expect(await screen.findByText(/Wird für Offline-Betrieb vorbereitet/)).toBeInTheDocument()
  })

  it("prompts a re-run after a service worker upgrade (stale)", async () => {
    mock("stale")
    render(<OfflineReadiness />)
    openStatus()
    expect(await screen.findByText("App wurde aktualisiert")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Erneut vorbereiten" })).toBeInTheDocument()
  })

  it("surfaces a retry on error", async () => {
    mock("error")
    render(<OfflineReadiness />)
    openStatus()
    expect(await screen.findByText("Vorbereitung fehlgeschlagen")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Erneut versuchen" })).toBeInTheDocument()
  })
})
