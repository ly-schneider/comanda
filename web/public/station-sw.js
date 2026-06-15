// Station PWA service worker.
//
// Caching split that the "Ready for offline" setup ritual relies on:
//   - App-shell document (`/station`): network-first, falls back to the cached
//     shell so the app boots with no signal.
//   - Hashed Next.js chunks (`/_next/static/*`), brand assets (`/assets/*`) +
//     the static shell list: cache-first; immutable, so a hit is always safe.
//   - Cross-origin product images (R2): served from cache when precached,
//     otherwise passed through. Fetched no-cors during precache so the opaque
//     response can be cached without R2 CORS headers.
//
// Structured event data (public key, catalog) lives in IndexedDB, not here, so
// it survives the cache wipe an SW version bump triggers on activate.

const VERSION = "comanda-station-v4"
const APP_DOC = "/station"
const SHELL = [
  "/station",
  "/station.webmanifest",
  "/icons/station-192.png",
  "/icons/station-512.png",
  "/apple-touch-icon.png",
  "/assets/fonts/Lovelo-Black.otf",
]

// Same-origin paths safe to serve cache-first: hashed Next.js build output and
// the immutable brand assets under /assets (e.g. the display font the station
// UI renders its headings in).
function isStaticAsset(pathname) {
  return pathname.startsWith("/_next/static/") || pathname.startsWith("/assets/")
}

function putInCache(request, res) {
  if (res && res.ok) {
    const copy = res.clone()
    caches.open(VERSION).then((c) => c.put(request, copy))
  }
}

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(VERSION)
      .then((c) => c.addAll(SHELL))
      .then(() => self.skipWaiting())
  )
})

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  )
})

async function cacheFirst(request) {
  const cached = await caches.match(request)
  if (cached) return cached
  const res = await fetch(request)
  putInCache(request, res)
  return res
}

async function networkFirst(request) {
  try {
    const res = await fetch(request)
    putInCache(request, res)
    return res
  } catch (err) {
    const cached = await caches.match(request)
    if (cached) return cached
    throw err
  }
}

self.addEventListener("fetch", (event) => {
  const request = event.request
  if (request.method !== "GET") return
  const url = new URL(request.url)

  // Cross-origin (product images): cache-only, no runtime population — those
  // are added deliberately by the precache ritual so we never balloon the cache.
  if (url.origin !== self.location.origin) {
    event.respondWith(caches.match(request, { ignoreVary: true }).then((c) => c || fetch(request)))
    return
  }

  if (url.pathname === APP_DOC) {
    event.respondWith(networkFirst(request))
    return
  }

  if (isStaticAsset(url.pathname) || SHELL.includes(url.pathname)) {
    event.respondWith(cacheFirst(request))
    return
  }

  // API calls and everything else hit the network directly.
})

async function precacheOne(cache, u) {
  try {
    const crossOrigin = new URL(u, self.location.origin).origin !== self.location.origin
    const request = new Request(u, crossOrigin ? { mode: "no-cors" } : { cache: "reload" })
    const res = await fetch(request)
    const ok = res && (res.ok || res.type === "opaque")
    if (ok) {
      await cache.put(u, res.clone())
      return { url: u, ok: true }
    }
    return { url: u, ok: false, status: res ? res.status : 0 }
  } catch (err) {
    return { url: u, ok: false, error: String((err && err.message) || err) }
  }
}

async function precache(urls) {
  const cache = await caches.open(VERSION)
  // Fetch concurrently; the browser's per-host connection pool bounds the real
  // parallelism, so a long shell + image list doesn't serialize end to end.
  return Promise.all(urls.map((u) => precacheOne(cache, u)))
}

async function cacheStatus(urls) {
  const cache = await caches.open(VERSION)
  const checks = await Promise.all(
    urls.map((u) => cache.match(u, { ignoreVary: true }).then((hit) => (hit ? null : u)))
  )
  return checks.filter((u) => u !== null)
}

self.addEventListener("message", (event) => {
  const data = event.data

  if (data === "hard-refresh") {
    event.waitUntil(
      caches
        .keys()
        .then((keys) => Promise.all(keys.map((k) => caches.delete(k))))
        .then(() => self.registration.unregister())
    )
    return
  }

  if (!data || typeof data !== "object") return
  const port = event.ports && event.ports[0]

  if (data.type === "station-sw-version") {
    if (port) port.postMessage({ type: "station-sw-version", version: VERSION })
    return
  }

  if (data.type === "station-precache") {
    const urls = Array.isArray(data.urls) ? data.urls : []
    event.waitUntil(
      precache(urls).then((results) => {
        if (port) port.postMessage({ type: "station-precache", results })
      })
    )
    return
  }

  if (data.type === "station-cache-status") {
    const urls = Array.isArray(data.urls) ? data.urls : []
    event.waitUntil(
      cacheStatus(urls).then((missing) => {
        if (port) port.postMessage({ type: "station-cache-status", version: VERSION, missing })
      })
    )
    return
  }
})
