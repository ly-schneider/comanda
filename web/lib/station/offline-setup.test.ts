import { beforeEach, describe, expect, it, vi } from "vitest"
import { memoryKVStore } from "@/lib/station/offline-db"
import {
  CATALOG_KEY,
  collectShellAssets,
  QR_CONFIG_KEY,
  READINESS_KEY,
  type ReadinessRecord,
  runOfflineSetup,
  type SetupDeps,
} from "@/lib/station/offline-setup"
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

function deps(overrides: Partial<SetupDeps> = {}, routes: Record<string, FakeResponse> = {}): SetupDeps {
  const table: Record<string, FakeResponse> = {
    "/api/v1/qr-config": QR_OK,
    "/api/v1/public/products": PRODUCTS_OK,
    "/api/v1/stations/me": STATION_ME_OK,
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

    // Image of the assigned product is precached through the SW.
    expect(d.sw.precache).toHaveBeenCalledWith(["/station", "/_next/static/a.js", "https://r2.example/p1.png"])
  })

  it("persists the public key, a lightweight catalog (no images), and the record", async () => {
    const d = deps()
    await runOfflineSetup(d)

    expect(await d.store.get(QR_CONFIG_KEY)).toMatchObject({ publicKey: "PK", payloadVersion: 1 })
    expect(await d.store.get(CATALOG_KEY)).toEqual([
      { id: "p1", name: "Pommes" },
      { id: "p2", name: "Cola" },
    ])
    const stored = (await d.store.get<ReadinessRecord>(READINESS_KEY))!
    expect(stored.ready).toBe(true)
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
    const d = deps({}, { "/api/v1/public/products": res(500, {}) })
    const record = await runOfflineSetup(d)

    expect(record.ready).toBe(false)
    expect(record.failures).toContain("/api/v1/public/products")
  })

  it("is not ready when a shell asset fails to cache", async () => {
    const d = deps({
      sw: {
        version: vi.fn(async () => "comanda-station-v4"),
        precache: vi.fn(async (urls: string[]) =>
          urls.map((u) => ({ url: u, ok: u !== "/_next/static/a.js", status: u !== "/_next/static/a.js" ? 200 : 404 }))
        ),
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

  it("includes the static shell, dedups same-origin chunks, and drops everything else", () => {
    const origin = window.location.origin
    addScript(`${origin}/_next/static/chunks/a.js`)
    addScript(`${origin}/_next/static/chunks/a.js`) // duplicate
    addScript("https://cdn.other.example/x.js") // cross-origin
    addScript(`${origin}/some/other.js`) // not a static chunk
    addStylesheet(`${origin}/_next/static/css/app.css`)

    const assets = collectShellAssets()

    expect(assets).toContain("/station")
    expect(assets).toContain("/station.webmanifest")
    expect(assets).toContain("/icons/station-192.png")
    expect(assets).toContain("/_next/static/chunks/a.js")
    expect(assets).toContain("/_next/static/css/app.css")
    expect(assets.filter((u) => u === "/_next/static/chunks/a.js")).toHaveLength(1)
    expect(assets.some((u) => u.includes("cdn.other.example"))).toBe(false)
    expect(assets).not.toContain("/some/other.js")
  })
})
