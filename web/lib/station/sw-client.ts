/**
 * Typed request/response bridge to the station service worker.
 *
 * Each call opens a one-shot MessageChannel and resolves on the worker's reply,
 * so callers get a promise instead of wiring up global `message` listeners.
 */

/** Scope the station service worker is registered under. */
export const STATION_SW_SCOPE = "/station"

const REPLY_TIMEOUT_MS = 30_000
const ACTIVATION_TIMEOUT_MS = 5_000

export interface PrecacheResult {
  url: string
  ok: boolean
  status?: number
  error?: string
}

export interface CacheStatus {
  version: string | null
  /** Subset of the queried urls that are NOT present in the live cache. */
  missing: string[]
}

export interface StationSWClient {
  /** Fetch + cache the given URLs in the worker; resolves with per-URL outcomes. */
  precache(urls: string[]): Promise<PrecacheResult[]>
  /** The active worker's cache version, or null if no worker controls the page. */
  version(): Promise<string | null>
  /** Which of the given urls are missing from the live cache, or null if no worker. */
  cacheStatus(urls: string[]): Promise<CacheStatus | null>
}

async function activeWorker(): Promise<ServiceWorker | null> {
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return null
  const reg = await navigator.serviceWorker.getRegistration(STATION_SW_SCOPE)
  if (reg?.active) return reg.active
  if (navigator.serviceWorker.controller) return navigator.serviceWorker.controller
  // Registered but not active yet (the window right after first registration).
  // Wait briefly for activation rather than failing the caller outright.
  return Promise.race([
    navigator.serviceWorker.ready.then((r) => r.active).catch(() => null),
    new Promise<null>((resolve) => setTimeout(() => resolve(null), ACTIVATION_TIMEOUT_MS)),
  ])
}

function request<T>(worker: ServiceWorker, message: unknown): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const channel = new MessageChannel()
    // Closing port1 tears down the channel and releases the transferred port2,
    // so a never-answered request doesn't leak a MessagePort pair.
    const close = () => {
      channel.port1.onmessage = null
      channel.port1.close()
    }
    const timer = setTimeout(() => {
      close()
      reject(new Error("service worker did not reply"))
    }, REPLY_TIMEOUT_MS)
    channel.port1.onmessage = (event) => {
      clearTimeout(timer)
      close()
      resolve(event.data as T)
    }
    worker.postMessage(message, [channel.port2])
  })
}

export function stationSWClient(): StationSWClient {
  return {
    async precache(urls) {
      const worker = await activeWorker()
      if (!worker) throw new Error("service worker unavailable")
      const reply = await request<{ results?: PrecacheResult[] }>(worker, {
        type: "station-precache",
        urls,
      })
      return reply.results ?? []
    },
    async version() {
      const worker = await activeWorker()
      if (!worker) return null
      try {
        const reply = await request<{ version?: string }>(worker, { type: "station-sw-version" })
        return reply.version ?? null
      } catch {
        return null
      }
    },
    async cacheStatus(urls) {
      const worker = await activeWorker()
      if (!worker) return null
      const reply = await request<{ version?: string; missing?: string[] }>(worker, {
        type: "station-cache-status",
        urls,
      })
      return { version: reply.version ?? null, missing: reply.missing ?? [] }
    },
  }
}
