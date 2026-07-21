import { beforeEach, describe, expect, it, vi } from "vitest"
import { memoryKVStore } from "@/lib/station/offline-db"
import {
  CATALOG_KEY,
  collectShellAssets,
  PLUGINS_KEY,
  QR_CONFIG_KEY,
  READINESS_KEY,
  type ReadinessRecord,
  refreshCatalog,
  runOfflineSetup,
  type SetupDeps,
} from "@/lib/station/offline-setup"
import { getCachedStationSession } from "@/lib/station/station-session"
import type { PrecacheResult } from "@/lib/station/sw-client"

type FakeResponse = { ok: boolean; status: number; json: () => Promise<unknown> }

function res(status: number, body: unknown): FakeResponse {
  return { ok: status >= 200 && status < 300, status, json: async () => body }
}

const QR_OK = res(200, { tenantId: "t1", alg: "ed25519", payloadVersion: 1, publicKey: "PK" })
const PRODUCTS_OK = res(200, {
  items: [
    { id: "p1", name: "Pommes", image: "https://r2.example/p1.png" },
    { id: "p2", name: "Cola", image: null },
  ],
})
const STATION_ME_OK = res(200, { products: [{ productId: "p1" }] })
const PLUGINS_OK = res(200, { items: [{ key: "artist-voucher", name: "Artist Voucher", config: {} }] })

function deps(overrides: Partial<SetupDeps> = {}, routes: Record<string, FakeResponse> = {}): SetupDeps {
  const table: Record<string, FakeResponse> = {
    "/api/v1/qr-config": QR_OK,
    "/api/v1/products": PRODUCTS_OK,
    "/api/v1/stations/me": STATION_ME_OK,
    "/api/v1/plugins": PLUGINS_OK,
    ...routes,
  }
  return {
    fetchFn: vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      const r = table[url]
      if (!r) throw new Error(`unexpected fetch: ${url}`)
      return r as unknown as Response
    }) as unknown as typeof fetch,
    store: memoryKVStore(),
    sw: {
      version: vi.fn(async () => "comanda-station-v4"),
      precache: vi.fn(async (urls: string[]): Promise<PrecacheResult[]> => urls.map((u) => ({ url: u, ok: true }))),
      cacheStatus: vi.fn(async () => ({ version: "comanda-station-v4", missing: [] })),
    },
    getToken: () => "device-token",
    collectAssets: () => ["/station", "/_next/static/a.js"],
    now: () => 1000,
    ...overrides,
  }
}

describe("runOfflineSetup", () => {
  it("becomes ready when key, catalog, shell and assigned images all cache", async () => {
    const d = deps()
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(true)
    expect(record.tenantId).toBe("t1")
    expect(record.payloadVersion).toBe(1)
    expect(record.publicKeyPresent).toBe(true)
    expect(record.swVersion).toBe("comanda-station-v4")
    expect(record.productIds).toEqual(["p1"])
    expect(record.shellCached).toBe(2)
    expect(record.imageCached).toBe(1)
    expect(record.failures).toEqual([])

    // The asset manifest is persisted so readiness can be re-verified against
    // the live cache on load.
    expect(record.assets).toEqual(["/station", "/_next/static/a.js", "https://r2.example/p1.png"])

    // Image of the assigned product is precached through the SW.
    expect(d.sw.precache).toHaveBeenCalledWith(["/station", "/_next/static/a.js", "https://r2.example/p1.png"])
  })

  it("persists the public key, a lightweight catalog (id, name, image url), and the record", async () => {
    const d = deps()
    await runOfflineSetup(d)

    expect(await d.store.get(QR_CONFIG_KEY)).toMatchObject({ publicKey: "PK", payloadVersion: 1 })
    expect(await d.store.get(CATALOG_KEY)).toEqual([
      { id: "p1", name: "Pommes", image: "https://r2.example/p1.png" },
      { id: "p2", name: "Cola", image: null },
    ])
    const stored = (await d.store.get<ReadinessRecord>(READINESS_KEY))!
    expect(stored.ready).toBe(true)
  })

  it("carries product variants into the catalog for offline variant rendering", async () => {
    const withVariants = res(200, {
      items: [
        {
          id: "p1",
          name: "Shirt",
          image: null,
          variants: [
            { id: "vrnt_size_xl", value: "XL" },
            { id: "vrnt_size_m0", value: "M" },
          ],
        },
      ],
    })
    const d = deps({}, { "/api/v1/products": withVariants })
    await runOfflineSetup(d)

    expect(await d.store.get(CATALOG_KEY)).toEqual([
      {
        id: "p1",
        name: "Shirt",
        image: null,
        variants: [
          { id: "vrnt_size_xl", value: "XL" },
          { id: "vrnt_size_m0", value: "M" },
        ],
      },
    ])
  })

  it("caches the enabled plugin keys for the offline voucher gate", async () => {
    const d = deps()
    await runOfflineSetup(d)
    expect(await d.store.get(PLUGINS_KEY)).toEqual(["artist-voucher"])
  })

  it("stays ready and caches nothing when the plugin list can't be loaded (vouchers optional)", async () => {
    const d = deps({}, { "/api/v1/plugins": res(500, {}) })
    const record = await runOfflineSetup(d)
    expect(record.ready).toBe(true)
    expect(record.failures).not.toContain("/api/v1/plugins")
    expect(await d.store.get(PLUGINS_KEY)).toBeNull()
  })

  it("caches the station session so the scanner can render offline after setup", async () => {
    const d = deps()
    await runOfflineSetup(d)

    expect(await getCachedStationSession(d.store)).toEqual({
      status: { products: [{ productId: "p1" }] },
      cachedAt: 1000,
    })
  })

  it("is not ready when the signing key is unprovisioned (503)", async () => {
    const d = deps({}, { "/api/v1/qr-config": res(503, { code: "qr_key_unprovisioned" }) })
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(false)
    expect(record.publicKeyPresent).toBe(false)
    expect(record.tenantId).toBeNull()
    expect(record.failures).toContain("/api/v1/qr-config")
  })

  it("is not ready when the catalog cannot be loaded", async () => {
    const d = deps({}, { "/api/v1/products": res(500, {}) })
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(false)
    expect(record.failures).toContain("/api/v1/products")
  })

  it("is not ready when the station identity request fails (e.g. expired token)", async () => {
    const d = deps({}, { "/api/v1/stations/me": res(401, {}) })
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(false)
    expect(record.failures).toContain("/api/v1/stations/me")
  })

  it("is not ready when a shell asset fails to cache", async () => {
    const d = deps({
      sw: {
        version: vi.fn(async () => "comanda-station-v4"),
        precache: vi.fn(async (urls: string[]) =>
          urls.map((u) => ({ url: u, ok: u !== "/_next/static/a.js", status: u !== "/_next/static/a.js" ? 200 : 404 }))
        ),
        cacheStatus: vi.fn(async () => ({ version: "comanda-station-v4", missing: [] })),
      },
    })
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(false)
    expect(record.failures).toContain("/_next/static/a.js")
  })

  it("is not ready when an assigned product image fails to cache", async () => {
    const d = deps({
      sw: {
        version: vi.fn(async () => "comanda-station-v4"),
        precache: vi.fn(async (urls: string[]) => urls.map((u) => ({ url: u, ok: !u.startsWith("https://") }))),
        cacheStatus: vi.fn(async () => ({ version: "comanda-station-v4", missing: [] })),
      },
    })
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(false)
    expect(record.failures).toContain("https://r2.example/p1.png")
  })

  it("caches shell + key + catalog without a device token (no images)", async () => {
    const d = deps({ getToken: () => null })
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(true)
    expect(record.productIds).toEqual([])
    expect(record.imageCached).toBe(0)
    expect(d.sw.precache).toHaveBeenCalledWith(["/station", "/_next/static/a.js"])
  })

  it("is not ready when no shell assets were collected", async () => {
    const d = deps({ collectAssets: () => [] })
    const record = await runOfflineSetup(d)
    expect(record.ready).toBe(false)
  })
})

describe("refreshCatalog", () => {
  it("rewrites only the catalog cache and touches no other endpoint", async () => {
    const d = deps()
    const ok = await refreshCatalog(d)

    expect(ok).toBe(true)
    expect(await d.store.get(CATALOG_KEY)).toEqual([
      { id: "p1", name: "Pommes", image: "https://r2.example/p1.png" },
      { id: "p2", name: "Cola", image: null },
    ])
    expect(d.fetchFn).toHaveBeenCalledTimes(1)
    expect(d.sw.precache).not.toHaveBeenCalled()
    expect(await d.store.get(READINESS_KEY)).toBeNull()
  })

  it("keeps the previous catalog when the fetch fails", async () => {
    const d = deps({}, { "/api/v1/products": res(500, {}) })
    await d.store.set(CATALOG_KEY, [{ id: "p1", name: "Pommes", image: null }])

    const ok = await refreshCatalog(d)

    expect(ok).toBe(false)
    expect(await d.store.get(CATALOG_KEY)).toEqual([{ id: "p1", name: "Pommes", image: null }])
  })
})

describe("collectShellAssets", () => {
  const added: Element[] = []

  beforeEach(() => {
    for (const el of added.splice(0)) el.remove()
  })

  function addScript(src: string) {
    const s = document.createElement("script")
    s.src = src
    document.head.appendChild(s)
    added.push(s)
  }
  function addStylesheet(href: string) {
    const l = document.createElement("link")
    l.rel = "stylesheet"
    l.href = href
    document.head.appendChild(l)
    added.push(l)
  }

  it("includes the static shell + brand assets, dedups same-origin chunks, and drops everything else", () => {
    const origin = window.location.origin
    addScript(`${origin}/_next/static/chunks/a.js`)
    addScript(`${origin}/_next/static/chunks/a.js`) // duplicate
    addScript("https://cdn.other.example/x.js") // cross-origin
    addScript(`${origin}/some/other.js`) // not a static chunk
    addStylesheet(`${origin}/_next/static/css/app.css`)
    addStylesheet(`${origin}/assets/brand.css`) // same-origin brand asset

    const assets = collectShellAssets()

    expect(assets).toContain("/station")
    expect(assets).toContain("/station.webmanifest")
    expect(assets).toContain("/icons/station-192.png")
    expect(assets).toContain("/assets/fonts/Lovelo-Black.otf")
    expect(assets).toContain("/_next/static/chunks/a.js")
    expect(assets).toContain("/_next/static/css/app.css")
    expect(assets).toContain("/assets/brand.css")
    expect(assets.filter((u) => u === "/_next/static/chunks/a.js")).toHaveLength(1)
    expect(assets.some((u) => u.includes("cdn.other.example"))).toBe(false)
    expect(assets).not.toContain("/some/other.js")
  })
})
