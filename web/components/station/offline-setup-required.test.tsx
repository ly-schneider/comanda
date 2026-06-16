import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"
import { OfflineSetupRequired } from "@/components/station/offline-setup-required"

afterEach(cleanup)

describe("OfflineSetupRequired", () => {
  it("tells the user to reconnect to finish setup", () => {
    render(<OfflineSetupRequired />)
    expect(screen.getByText("Offline – Einrichtung nötig")).toBeInTheDocument()
    expect(screen.getByText(/Verbinde dich mit dem Internet/)).toBeInTheDocument()
  })
})
