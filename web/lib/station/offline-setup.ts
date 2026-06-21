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
import { resolveStationSession, STATION_ME_URL } from "@/lib/station/station-session"
import { type PrecacheResult, type StationSWClient, stationSWClient } from "@/lib/station/sw-client"

export const READINESS_KEY = "readiness"
export const QR_CONFIG_KEY = "qrConfig"
export const CATALOG_KEY = "catalog"

const QR_CONFIG_URL = "/api/v1/qr-config"
const CATALOG_URL = "/api/v1/products"

// Same-origin paths whose contents are immutable enough to cache-first and
// precache: hashed Next.js build output and the static brand assets under
// /assets (e.g. the display font the station UI renders headings in).
const SHELL_ASSET_PREFIXES = ["/_next/static/", "/assets/"]

const STATIC_SHELL = [
  "/station",
  "/station.webmanifest",
  "/icons/station-192.png",
  "/icons/station-512.png",
  "/apple-touch-icon.png",
  "/assets/fonts/Lovelo-Black.otf",
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
  image?: string | null
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
  // Every asset this record asserts is cached. Used to re-verify against the
  // live SW cache on load — the cache is wiped on every SW version bump, while
  // this record (in IndexedDB) survives, so "ready" must be re-checked, not
  // trusted blindly.
  assets: string[]
}

interface ProductDTO {
  id: string
  name: string
  image?: string | null
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
 * list plus every Next.js chunk and brand asset already loaded into this
 * document. Restricting to `SHELL_ASSET_PREFIXES` keeps analytics and other
 * third-party requests out of the cache. Robust on first load too, where the
 * SW didn't yet intercept chunks.
 */
export function collectShellAssets(): string[] {
  const urls = new Set<string>(STATIC_SHELL)
  if (typeof document === "undefined") return Array.from(urls)

  const origin = window.location.origin
  const addSameOriginStatic = (raw: string | null | undefined) => {
    if (!raw) return
    try {
      const u = new URL(raw, origin)
      if (u.origin === origin && SHELL_ASSET_PREFIXES.some((p) => u.pathname.startsWith(p))) {
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
  // Carry the image URL too: the bytes are SW-precached separately, but the
  // station needs the URL to render the scan card fully offline.
  const entries: CatalogEntry[] = products.map((p) => ({ id: p.id, name: p.name, image: p.image ?? null }))
  await deps.store.set<CatalogEntry[]>(CATALOG_KEY, entries)
  return { entries, products }
}

/**
 * Resolves the station's assigned product ids through the session module — the
 * single owner of the cached identity — so the scanner can render offline
 * without a `/stations/me` round-trip and revocation clears the cache the same
 * way it does at runtime. `ok` is false only when a token was present but the
 * identity couldn't be resolved (an expired/revoked token must block readiness
 * rather than masquerade as a station with no assignments — the legitimate empty
 * case is no token, or an empty product list).
 */
async function loadAssignedProductIds(deps: SetupDeps): Promise<{ ids: string[]; ok: boolean }> {
  const token = deps.getToken()
  if (!token) return { ids: [], ok: true }
  const resolution = await resolveStationSession({
    token,
    online: true,
    fetchFn: deps.fetchFn,
    store: deps.store,
    now: deps.now,
  })
  if (resolution.kind !== "resolved") return { ids: [], ok: false }
  return { ids: (resolution.status.products ?? []).map((p) => p.productId), ok: true }
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

  // None of these depend on each other, so fire them concurrently.
  const [qrConfig, catalog, assigned] = await Promise.all([
    loadQRConfig(deps).catch(() => null),
    loadCatalog(deps).catch(() => null),
    loadAssignedProductIds(deps).catch(() => ({ ids: [] as string[], ok: false })),
  ])
  if (!qrConfig) failures.push(QR_CONFIG_URL)
  if (!catalog) failures.push(CATALOG_URL)
  if (!assigned.ok) failures.push(STATION_ME_URL)

  const assignedIds = assigned.ids
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
    assets: ready ? precacheList : [],
  }
  await deps.store.set<ReadinessRecord>(READINESS_KEY, record)
  return record
}

export function getReadiness(store: StationKVStore = idbKVStore()): Promise<ReadinessRecord | null> {
  return store.get<ReadinessRecord>(READINESS_KEY)
}
