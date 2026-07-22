import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { AppVersionButton } from "@/components/pos/app-version-button"

type Bridge = { getAppVersion?: () => string; checkForUpdate?: () => void }

function setBridge(bridge: Bridge | undefined) {
  ;(globalThis as unknown as { PosBridge?: Bridge }).PosBridge = bridge
}

afterEach(() => {
  cleanup()
  setBridge(undefined)
})

describe("AppVersionButton", () => {
  it("renders nothing when no native bridge is present", () => {
    setBridge(undefined)
    const { container } = render(<AppVersionButton />)
    expect(container).toBeEmptyDOMElement()
  })

  it("renders nothing when getAppVersion returns malformed JSON", () => {
    setBridge({ getAppVersion: () => "not json{" })
    const { container } = render(<AppVersionButton />)
    expect(container).toBeEmptyDOMElement()
  })

  it("renders nothing when the parsed payload lacks versionName", () => {
    setBridge({ getAppVersion: () => JSON.stringify({ versionCode: 10007 }) })
    const { container } = render(<AppVersionButton />)
    expect(container).toBeEmptyDOMElement()
  })

  it("shows the version and disables update when checkForUpdate is missing", () => {
    setBridge({ getAppVersion: () => JSON.stringify({ versionName: "1.0.7", versionCode: 10007 }) })
    render(<AppVersionButton />)

    fireEvent.click(screen.getByLabelText("App-Version"))
    expect(screen.getByText("Version 1.0.7")).toBeInTheDocument()
    expect(screen.getByText("Build 10007")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /Nach Updates suchen/ })).toBeDisabled()
  })

  it("triggers checkForUpdate when the bridge supports it", () => {
    const checkForUpdate = vi.fn()
    setBridge({
      getAppVersion: () => JSON.stringify({ versionName: "1.0.7", versionCode: 10007 }),
      checkForUpdate,
    })
    render(<AppVersionButton />)

    fireEvent.click(screen.getByLabelText("App-Version"))
    fireEvent.click(screen.getByRole("button", { name: /Nach Updates suchen/ }))
    expect(checkForUpdate).toHaveBeenCalledTimes(1)
  })
})
