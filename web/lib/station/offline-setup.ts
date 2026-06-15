/**
 * The explicit "Ready for offline" setup ritual.
 *
 * Run once while online: it caches the app shell + Next.js chunks, stores the
 * tenant public key and lightweight catalog in IndexedDB, and precaches the
 * assigned product images. Only when every required asset lands does it persist
 * a readiness record — so the UI can show "Ready for offline" truthfully.
 *
 * Side effects are injected (fetch, store, SW client, token, asset list) so the
 * orchestration is unit-testable without a browser.
 */

import { getDeviceToken } from "@/lib/device-auth"
import { idbKVStore, type StationKVStore } from "@/lib/station/offline-db"
import { type PrecacheResult, type StationSWClient, stationSWClient } from "@/lib/station/sw-client"

export const READINESS_KEY = "readiness"
export const QR_CONFIG_KEY = "qrConfig"
export const CATALOG_KEY = "catalog"

const QR_CONFIG_URL = "/api/v1/qr-config"
const CATALOG_URL = "/api/v1/public/products"
const STATION_ME_URL = "/api/v1/stations/me"

const STATIC_SHELL = [
  "/station",
  "/station.webmanifest",
  "/icons/station-192.png",
  "/icons/station-512.png",
  "/apple-touch-icon.png",
]

export interface QRConfig {
  tenantId: string
  alg: string
  payloadVersion: number
  publicKey: string
}

export interface CatalogEntry {
  id: string
  name: string
}

export interface ReadinessRecord {
  ready: boolean
  completedAt: number
  swVersion: string | null
  tenantId: string | null
  payloadVersion: number | null
  publicKeyPresent: boolean
  productIds: string[]
  shellCached: number
  imageCached: number
  failures: string[]
}

interface ProductDTO {
  id: string
  name: string
  image?: string | null
}

interface StationMeDTO {
  products?: { productId: string }[]
}

export interface SetupDeps {
  fetchFn: typeof fetch
  store: StationKVStore
  sw: StationSWClient
  getToken: () => string | null
  collectAssets: () => string[]
  now: () => number
}

/**
 * Same-origin shell assets the booted `/station` page depends on: the static
 * list plus every Next.js chunk already loaded into this document. Restricting
 * to `/_next/static/` keeps analytics and other third-party requests out of the
 * cache. Robust on first load too, where the SW didn't yet intercept chunks.
 */
export function collectShellAssets(): string[] {
  const urls = new Set<string>(STATIC_SHELL)
  if (typeof document === "undefined") return Array.from(urls)

  const origin = window.location.origin
  const addSameOriginStatic = (raw: string | null | undefined) => {
    if (!raw) return
    try {
      const u = new URL(raw, origin)
      if (u.origin === origin && u.pathname.startsWith("/_next/static/")) {
        urls.add(u.pathname + u.search)
      }
    } catch {
      // ignore unparseable URLs
    }
  }

  document.querySelectorAll<HTMLScriptElement>("script[src]").forEach((s) => addSameOriginStatic(s.src))
  document
    .querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"][href], link[rel="preload"][href]')
    .forEach((l) => addSameOriginStatic(l.href))

  if (typeof performance !== "undefined" && performance.getEntriesByType) {
    for (const entry of performance.getEntriesByType("resource")) {
      addSameOriginStatic(entry.name)
    }
  }

  return Array.from(urls)
}

async function loadQRConfig(deps: SetupDeps): Promise<QRConfig | null> {
  const res = await deps.fetchFn(QR_CONFIG_URL)
  if (!res.ok) return null
  const cfg = (await res.json()) as QRConfig
  if (!cfg.publicKey) return null
  await deps.store.set<QRConfig>(QR_CONFIG_KEY, cfg)
  return cfg
}

async function loadCatalog(deps: SetupDeps): Promise<{ entries: CatalogEntry[]; products: ProductDTO[] } | null> {
  const res = await deps.fetchFn(CATALOG_URL)
  if (!res.ok) return null
  const body = (await res.json()) as { items?: ProductDTO[] }
  const products = body.items ?? []
  const entries: CatalogEntry[] = products.map((p) => ({ id: p.id, name: p.name }))
  await deps.store.set<CatalogEntry[]>(CATALOG_KEY, entries)
  return { entries, products }
}

async function loadAssignedProductIds(deps: SetupDeps): Promise<string[]> {
  const token = deps.getToken()
  if (!token) return []
  const res = await deps.fetchFn(STATION_ME_URL, { headers: { Authorization: `Bearer ${token}` } })
  if (!res.ok) return []
  const body = (await res.json()) as StationMeDTO
  return (body.products ?? []).map((p) => p.productId)
}

function buildDefaultDeps(overrides?: Partial<SetupDeps>): SetupDeps {
  return {
    fetchFn: overrides?.fetchFn ?? ((...args) => fetch(...args)),
    store: overrides?.store ?? idbKVStore(),
    sw: overrides?.sw ?? stationSWClient(),
    getToken: overrides?.getToken ?? getDeviceToken,
    collectAssets: overrides?.collectAssets ?? collectShellAssets,
    now: overrides?.now ?? (() => Date.now()),
  }
}

export async function runOfflineSetup(overrides?: Partial<SetupDeps>): Promise<ReadinessRecord> {
  const deps = buildDefaultDeps(overrides)
  const failures: string[] = []

  const swVersion = await deps.sw.version()

  const [qrConfig, catalog] = await Promise.all([
    loadQRConfig(deps).catch(() => null),
    loadCatalog(deps).catch(() => null),
  ])
  if (!qrConfig) failures.push(QR_CONFIG_URL)
  if (!catalog) failures.push(CATALOG_URL)

  const assignedIds = await loadAssignedProductIds(deps).catch(() => [])
  const assignedSet = new Set(assignedIds)
  const imageUrls = (catalog?.products ?? [])
    .filter((p) => assignedSet.has(p.id) && !!p.image)
    .map((p) => p.image as string)

  const shellAssets = deps.collectAssets()
  const precacheList = Array.from(new Set([...shellAssets, ...imageUrls]))

  let results: PrecacheResult[] = []
  if (precacheList.length > 0) {
    try {
      results = await deps.sw.precache(precacheList)
    } catch (err) {
      failures.push(`precache: ${err instanceof Error ? err.message : String(err)}`)
    }
  }

  const imageSet = new Set(imageUrls)
  let shellCached = 0
  let imageCached = 0
  for (const r of results) {
    if (!r.ok) {
      failures.push(r.url)
      continue
    }
    if (imageSet.has(r.url)) imageCached++
    else shellCached++
  }

  const shellOk = shellAssets.length > 0 && shellCached >= shellAssets.length
  const ready = failures.length === 0 && !!qrConfig && !!catalog && shellOk

  const record: ReadinessRecord = {
    ready,
    completedAt: deps.now(),
    swVersion,
    tenantId: qrConfig?.tenantId ?? null,
    payloadVersion: qrConfig?.payloadVersion ?? null,
    publicKeyPresent: !!qrConfig?.publicKey,
    productIds: assignedIds,
    shellCached,
    imageCached,
    failures,
  }
  await deps.store.set<ReadinessRecord>(READINESS_KEY, record)
  return record
}

export function getReadiness(store: StationKVStore = idbKVStore()): Promise<ReadinessRecord | null> {
  return store.get<ReadinessRecord>(READINESS_KEY)
}
