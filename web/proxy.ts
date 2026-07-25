import { NextRequest, NextResponse } from "next/server"
import { isDevEnv, isProdEnv } from "@/lib/env"
import { getPublicHost, parseTenantHost } from "@/lib/tenant"

/**
 * Build a mutated request-headers object that pins x-comanda-host.
 * Next.js middleware can override request headers by returning them from
 * NextResponse.next({ request: { headers } }).
 */
function buildRequestHeaders(request: NextRequest, publicHost: string): Headers {
  const headers = new Headers(request.headers)
  if (publicHost) {
    headers.set("x-comanda-host", publicHost)
  }
  return headers
}

const HTTPS_UPGRADE_COOKIE = "comanda-https-upgrade"

/**
 * The scheme the *browser* used, or null when it can't be established.
 *
 * A custom domain behind the customer's Cloudflare is https to the browser but
 * plain :80 to Traefik, which overwrites x-forwarded-proto. Only CF-Visitor
 * survives with the visitor's own scheme. Null over a guess — a wrong "http"
 * verdict redirects forever.
 */
function visitorScheme(request: NextRequest): "http" | "https" | null {
  const cfVisitor = request.headers.get("cf-visitor")
  if (cfVisitor) {
    try {
      const scheme = String((JSON.parse(cfVisitor) as { scheme?: string })?.scheme || "").toLowerCase()
      if (scheme === "http" || scheme === "https") return scheme
    } catch {}
    return null
  }
  if (request.headers.get("cf-ray")) return null

  const proto = (request.headers.get("x-forwarded-proto") || "").split(",")[0]!.trim().toLowerCase()
  return proto === "http" || proto === "https" ? proto : null
}

function httpsUpgrade(request: NextRequest, publicHost: string, pathname: string): NextResponse | null {
  if (isDevEnv() || visitorScheme(request) !== "http") return null
  if (pathname.startsWith("/.well-known/acme-challenge/")) return null
  // Loop breaker: an unrecognised TLS terminator in front is indistinguishable
  // from a plain http visitor. One upgrade, then serve.
  if (request.cookies.get(HTTPS_UPGRADE_COOKIE)) return null

  const url = new URL(request.nextUrl)
  url.protocol = "https:"
  url.port = ""
  if (publicHost) url.host = publicHost

  const response = NextResponse.redirect(url, 308)
  response.cookies.set({
    name: HTTPS_UPGRADE_COOKIE,
    value: "1",
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    maxAge: 10,
  })
  return response
}

export async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl

  // Resolve the canonical public host and classify tenant context.
  // No DB call here — purely string-based. The backend is the authoritative
  // resolver and returns 404 for unknown hosts.
  const publicHost = getPublicHost(request)
  const tenantResult = parseTenantHost(publicHost)

  // Custom-domain Ingresses carry no Traefik redirect-https middleware (an
  // origin-level redirect loops there), so the upgrade happens here.
  const upgrade = httpsUpgrade(request, publicHost, pathname)
  if (upgrade) return upgrade

  const isStatic = pathname.startsWith("/_next/") || pathname.includes(".")
  // Dev-only: per-request IP/UA log, too noisy for staging/prod.
  if (!isStatic && process.env.NODE_ENV === "development") {
    try {
      const method = request.method
      const ua = request.headers.get("user-agent") || "-"
      const fwd = request.headers.get("x-forwarded-for") || "-"
      const proto = (request.headers.get("x-forwarded-proto") || "").toLowerCase()
      console.log(
        `[req] ${method} ${pathname} tenant=${tenantResult.type}${"slug" in tenantResult ? `(${tenantResult.slug})` : ""} proto=${proto || "-"} ip=${fwd.split(",")[0]?.trim() || "-"} ua=${ua.substring(0, 80)}`
      )
    } catch {}
  }

  if (isStatic || pathname.startsWith("/api/")) {
    // Forward x-comanda-host on API routes so the backend proxy can pick it up.
    return NextResponse.next({
      request: { headers: buildRequestHeaders(request, publicHost) },
    })
  }

  // The platform-admin control plane and the vendor account hub (/account
  // redirector + /t/<tenant>) live only on the platform host; on a tenant or
  // custom-domain host they do not exist.
  if (
    (pathname.startsWith("/platform-admin") || pathname.startsWith("/account") || pathname.startsWith("/t/")) &&
    tenantResult.type !== "platform"
  ) {
    const url = request.nextUrl.clone()
    url.pathname = "/"
    url.search = ""
    return NextResponse.redirect(url)
  }

  // Checkout and Orders are intentionally NOT protected to allow guest flows
  const protectedPrefixes = ["/profile", "/admin", "/platform-admin", "/account", "/t/"]
  const isProtected = protectedPrefixes.some((p) => pathname.startsWith(p))
  if (isProtected) {
    // Check both names: x-forwarded-proto can disagree with the cookie's prefix
    // on a custom domain. This is presence-only; real validation is downstream.
    const hasSession =
      request.cookies.get("__Secure-better-auth.session_token")?.value ||
      request.cookies.get("better-auth.session_token")?.value
    if (!hasSession) {
      const url = request.nextUrl.clone()
      url.pathname = "/login"
      url.searchParams.set("next", request.nextUrl.pathname)
      return NextResponse.redirect(url)
    }
  }

  const response = NextResponse.next({
    request: { headers: buildRequestHeaders(request, publicHost) },
  })

  response.headers.set("X-Frame-Options", "DENY")
  response.headers.set("X-Content-Type-Options", "nosniff")
  response.headers.set("Referrer-Policy", "origin-when-cross-origin")

  // No includeSubDomains — a BYO apex has subdomains the platform doesn't serve.
  if (visitorScheme(request) === "https") {
    response.headers.set("Strict-Transport-Security", "max-age=15552000")
  }

  if (!isProdEnv()) {
    response.headers.set("X-Robots-Tag", "noindex, nofollow")
  }

  const umamiScript = "https://cloud.umami.is"
  const umamiConnect =
    "https://api-gateway.umami.dev https://api-gateway-eu.umami.dev https://gateway.umami.is https://eu.umami.is"

  const csp = [
    "default-src 'self'",
    `script-src 'self' 'unsafe-inline' 'unsafe-eval' ${umamiScript}`,
    "style-src 'self' 'unsafe-inline'",
    `img-src 'self' data: blob: https:${process.env.NODE_ENV === "development" ? " http://localhost:* http://127.0.0.1:*" : ""}`,
    "font-src 'self' data:",
    `connect-src 'self' ws: wss: ${umamiConnect}`,
    "frame-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
  ].join("; ")

  response.headers.set("Content-Security-Policy", csp)

  try {
    // Key the name off env, not x-forwarded-proto: a custom domain's edge→origin
    // hop is http, but the https browser (lib/csrf.ts) reads __Host-csrf.
    const secure = !isDevEnv()
    const csrfName = secure ? "__Host-csrf" : "csrf"
    const hasCsrf = request.cookies.get(csrfName)?.value
    if (!hasCsrf) {
      const bytes = crypto.getRandomValues(new Uint8Array(16))
      const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
      let token = ""
      for (let i = 0; i < bytes.length; i++) token += chars[bytes[i]! % chars.length]
      response.cookies.set({
        name: csrfName,
        value: token,
        httpOnly: false,
        secure,
        sameSite: "lax",
        path: "/",
        maxAge: 7 * 24 * 60 * 60,
      })
    }
  } catch {}

  return response
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|.*\\.png$|.*\\.jpg$|.*\\.jpeg$|.*\\.gif$|.*\\.svg$).*)"],
}
