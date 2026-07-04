import { beforeEach, describe, expect, it, vi } from "vitest"

vi.mock("@/lib/server/host", () => ({ getRequestHost: vi.fn(async () => "shop.comanda.test") }))

const apiRequest = vi.fn()
vi.mock("@/lib/api", () => ({ apiRequest: (...args: unknown[]) => apiRequest(...args) }))

// React cache() may memoize across cases; re-import the module per test.
async function loadModule() {
  vi.resetModules()
  return import("@/lib/server/branding")
}

describe("branding suspension inference", () => {
  beforeEach(() => {
    apiRequest.mockReset()
  })

  it("returns backend branding and not suspended on success", async () => {
    apiRequest.mockResolvedValue({ displayName: "Acme", colors: { primary: "#123456" } })
    const { getBranding, isTenantSuspended } = await loadModule()

    const branding = await getBranding()
    expect(branding.displayName).toBe("Acme")
    expect(branding.colors.primary).toBe("#123456")
    expect(branding.colors.background).toBe("#ffffff")
    expect(await isTenantSuspended()).toBe(false)
  })

  it("treats a 403 as suspended and falls back to defaults", async () => {
    apiRequest.mockRejectedValue({ status: 403, message: "This tenant is suspended." })
    const { getBranding, isTenantSuspended } = await loadModule()

    expect(await isTenantSuspended()).toBe(true)
    expect((await getBranding()).displayName).toBe("Comanda")
  })

  it("does not treat other failures as suspended", async () => {
    apiRequest.mockRejectedValue({ status: 500, message: "boom" })
    const { getBranding, isTenantSuspended } = await loadModule()

    expect(await isTenantSuspended()).toBe(false)
    expect((await getBranding()).displayName).toBe("Comanda")
  })

  it("does not treat non-API errors as suspended", async () => {
    apiRequest.mockRejectedValue(new Error("network down"))
    const { isTenantSuspended } = await loadModule()

    expect(await isTenantSuspended()).toBe(false)
  })
})
