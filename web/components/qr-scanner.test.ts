import { describe, expect, it, vi } from "vitest"
import { enableContinuousFocus } from "./qr-scanner"

// Stub the wasm reader so importing the module doesn't touch zxing-wasm.
vi.mock("zxing-wasm/reader", () => ({
  prepareZXingModule: vi.fn(),
  readBarcodes: vi.fn(),
}))

function trackWith(getCapabilities: (() => unknown) | undefined) {
  const applyConstraints = vi.fn().mockResolvedValue(undefined)
  const track = { getCapabilities, applyConstraints } as unknown as MediaStreamTrack
  return { track, applyConstraints }
}

describe("enableContinuousFocus", () => {
  it("applies continuous focus when the track supports it", async () => {
    const { track, applyConstraints } = trackWith(() => ({ focusMode: ["manual", "continuous"] }))
    await enableContinuousFocus(track)
    expect(applyConstraints).toHaveBeenCalledWith({ advanced: [{ focusMode: "continuous" }] })
  })

  it("is a no-op when the track reports no focusMode capability", async () => {
    const { track, applyConstraints } = trackWith(() => ({}))
    await enableContinuousFocus(track)
    expect(applyConstraints).not.toHaveBeenCalled()
  })

  it("is a no-op when continuous is not among the supported modes", async () => {
    const { track, applyConstraints } = trackWith(() => ({ focusMode: ["manual"] }))
    await enableContinuousFocus(track)
    expect(applyConstraints).not.toHaveBeenCalled()
  })

  it("is a no-op when getCapabilities is unavailable", async () => {
    const { track, applyConstraints } = trackWith(undefined)
    await expect(enableContinuousFocus(track)).resolves.toBeUndefined()
    expect(applyConstraints).not.toHaveBeenCalled()
  })

  it("swallows a rejected applyConstraints", async () => {
    const applyConstraints = vi.fn().mockRejectedValue(new Error("OverconstrainedError"))
    const track = {
      getCapabilities: () => ({ focusMode: ["continuous"] }),
      applyConstraints,
    } as unknown as MediaStreamTrack
    await expect(enableContinuousFocus(track)).resolves.toBeUndefined()
    expect(applyConstraints).toHaveBeenCalled()
  })

  it("swallows a throwing getCapabilities", async () => {
    const applyConstraints = vi.fn()
    const track = {
      getCapabilities: () => {
        throw new Error("boom")
      },
      applyConstraints,
    } as unknown as MediaStreamTrack
    await expect(enableContinuousFocus(track)).resolves.toBeUndefined()
    expect(applyConstraints).not.toHaveBeenCalled()
  })
})
