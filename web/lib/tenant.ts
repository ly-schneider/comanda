/**
 * Tenant host resolution utilities.
 *
 * Classifies an incoming hostname as either a platform host (no tenant
 * context), a tenant subdomain (slug extracted), or a custom domain
 * (passed through — the backend resolves it against tenant_domain).
 *
 * No DB calls here; classification is purely string-based. The backend
 * is the authoritative resolver and returns 404 for unknown hosts.
 */

/** Subdomains that are reserved for platform use and must not resolve as tenant slugs. */
const RESERVED_SLUGS = new Set([
  "www",
  "api",
  "app",
  "beta",
  "static",
  "assets",
  "cdn",
  "mail",
  "smtp",
  "admin",
  "platform",
])

/** Platform-level bare hostnames (no tenant context). */
const PLATFORM_BARE_HOSTS = new Set(["localhost", "127.0.0.1", "comanda.leys.ch", "beta.comanda.leys.ch"])

export type TenantHostResult =
  | { type: "platform" }
  | { type: "tenant-subdomain"; slug: string }
  | { type: "custom-domain" }

// First entry of a comma-list, port stripped, lowercased. Empty in → empty out.
export function normalizeHost(raw: string | null | undefined): string {
  return (raw || "").split(",")[0]!.trim().split(":")[0]!.toLowerCase()
}

/**
 * Classify a bare hostname (no port, lowercased) into platform / tenant-subdomain / custom-domain.
 *
 * Recognised patterns:
 *   - `comanda.leys.ch`, `beta.comanda.leys.ch`, `localhost`, `127.0.0.1`  → platform
 *   - `<slug>.comanda.leys.ch`, `<slug>.beta.comanda.leys.ch`              → tenant-subdomain
 *   - `<slug>.localhost`                                                    → tenant-subdomain (dev)
 *   - anything else                                                         → custom-domain
 */
export function parseTenantHost(hostname: string): TenantHostResult {
  const host = normalizeHost(hostname)

  if (PLATFORM_BARE_HOSTS.has(host)) {
    return { type: "platform" }
  }

  // Extra platform hosts supplied via env (comma-separated).
  const extraPlatformHosts = (process.env.PLATFORM_HOSTS || "")
    .split(",")
    .map((h) => h.trim().toLowerCase())
    .filter(Boolean)
  if (extraPlatformHosts.includes(host)) {
    return { type: "platform" }
  }

  // Check the more-specific beta staging apex BEFORE the prod apex, because
  // acme.beta.comanda.leys.ch also matches *.comanda.leys.ch.

  // Tenant subdomains on the beta staging apex.
  if (host.endsWith(".beta.comanda.leys.ch")) {
    const slug = host.replace(/\.beta\.comanda\.leys\.ch$/, "")
    if (!slug.includes(".") && !RESERVED_SLUGS.has(slug) && slug.length > 0) {
      return { type: "tenant-subdomain", slug }
    }
    return { type: "platform" }
  }

  // Tenant subdomains on the comanda.leys.ch apex.
  if (host.endsWith(".comanda.leys.ch")) {
    const slug = host.replace(/\.comanda\.leys\.ch$/, "")
    // Reject multi-level subdomains and reserved names.
    if (!slug.includes(".") && !RESERVED_SLUGS.has(slug) && slug.length > 0) {
      return { type: "tenant-subdomain", slug }
    }
    return { type: "platform" }
  }

  // Dev: <slug>.localhost
  if (host.endsWith(".localhost")) {
    const slug = host.replace(/\.localhost$/, "")
    if (!slug.includes(".") && !RESERVED_SLUGS.has(slug) && slug.length > 0) {
      return { type: "tenant-subdomain", slug }
    }
    return { type: "platform" }
  }

  return { type: "custom-domain" }
}

/**
 * Extract the canonical public host from an incoming request.
 *
 * Respects X-Forwarded-Host (set by the load balancer / Traefik in prod),
 * falls back to the Host header. Strips port, takes first of comma-lists,
 * lowercases.
 */
export function getPublicHost(request: Request): string {
  const headers = request.headers
  return normalizeHost(headers.get("x-forwarded-host") || headers.get("host"))
}
