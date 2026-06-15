/**
 * Typed request/response bridge to the station service worker.
 *
 * Each call opens a one-shot MessageChannel and resolves on the worker's reply,
 * so callers get a promise instead of wiring up global `message` listeners.
 */

const SCOPE = "/station"
const REPLY_TIMEOUT_MS = 30_000

export interface PrecacheResult {
  url: string
  ok: boolean
  status?: number
  error?: string
}

export interface StationSWClient {
  /** Fetch + cache the given URLs in the worker; resolves with per-URL outcomes. */
  precache(urls: string[]): Promise<PrecacheResult[]>
  /** The active worker's cache version, or null if no worker controls the page. */
  version(): Promise<string | null>
}

async function activeWorker(): Promise<ServiceWorker | null> {
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return null
  const reg = await navigator.serviceWorker.getRegistration(SCOPE)
  return reg?.active ?? navigator.serviceWorker.controller ?? null
}

function request<T>(worker: ServiceWorker, message: unknown): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const channel = new MessageChannel()
    const timer = setTimeout(() => {
      channel.port1.onmessage = null
      reject(new Error("service worker did not reply"))
    }, REPLY_TIMEOUT_MS)
    channel.port1.onmessage = (event) => {
      clearTimeout(timer)
      channel.port1.onmessage = null
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
  }
}
