import { beforeEach, describe, expect, it, vi } from "vitest"

const getRequestHost = vi.fn(async () => "shop.comanda.test")
vi.mock("@/lib/server/host", () => ({ getRequestHost: () => getRequestHost() }))

const apiRequest = vi.fn()
vi.mock("@/lib/api", () => ({ apiRequest: (...args: unknown[]) => apiRequest(...args) }))

// React cache() may memoize across cases; re-import the module per test.
async function loadModule() {
  vi.resetModules()
  return import("@/lib/server/plugins")
}

describe("getPlugins", () => {
  beforeEach(() => {
    apiRequest.mockReset()
    getRequestHost.mockReset()
    getRequestHost.mockResolvedValue("shop.comanda.test")
  })

  it("returns the enabled plugins and forwards the resolved host", async () => {
    apiRequest.mockResolvedValue({ items: [{ key: "club100", name: "100 Club", config: { max: 3 } }] })
    const { getPlugins } = await loadModule()

    const plugins = await getPlugins()
    expect(plugins).toEqual([{ key: "club100", name: "100 Club", config: { max: 3 } }])
    expect(apiRequest).toHaveBeenCalledWith("/v1/plugins", { headers: { "X-Forwarded-Host": "shop.comanda.test" } })
  })

  it("omits the host header when no host resolves", async () => {
    getRequestHost.mockResolvedValue("")
    apiRequest.mockResolvedValue({ items: [] })
    const { getPlugins } = await loadModule()

    await getPlugins()
    expect(apiRequest).toHaveBeenCalledWith("/v1/plugins", { headers: {} })
  })

  it("falls back to an empty list when the body has no items", async () => {
    apiRequest.mockResolvedValue({})
    const { getPlugins } = await loadModule()

    expect(await getPlugins()).toEqual([])
  })

  it("falls back to an empty list on any request failure", async () => {
    apiRequest.mockRejectedValue({ status: 500, message: "boom" })
    const { getPlugins } = await loadModule()

    expect(await getPlugins()).toEqual([])
  })
})
