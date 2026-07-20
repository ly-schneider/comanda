import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { OrderScanner } from "@/components/pos/order-scanner"

// Stand in for the camera scanner: a button that fires onResult, so a "scan" is a
// click. Avoids getUserMedia/zxing-wasm entirely.
vi.mock("@/components/qr-scanner", () => ({
  default: ({ onResult }: { onResult: (r: { text: string }) => void }) => (
    <button onClick={() => onResult({ text: "tok" })}>fire</button>
  ),
}))

afterEach(cleanup)

describe("OrderScanner lock", () => {
  it("locks after one scan but resets when the dialog is reopened", () => {
    const onScan = vi.fn()
    const { rerender } = render(<OrderScanner open onOpenChange={() => {}} onScan={onScan} />)

    fireEvent.click(screen.getByText("fire"))
    fireEvent.click(screen.getByText("fire"))
    expect(onScan).toHaveBeenCalledTimes(1) // second scan swallowed while open

    // Auto-close on a hit is a controlled prop change (no onOpenChange), then reopen.
    rerender(<OrderScanner open={false} onOpenChange={() => {}} onScan={onScan} />)
    rerender(<OrderScanner open onOpenChange={() => {}} onScan={onScan} />)

    fireEvent.click(screen.getByText("fire"))
    expect(onScan).toHaveBeenCalledTimes(2) // lock reset — scanning works again
  })
})
