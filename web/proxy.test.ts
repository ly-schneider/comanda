import { NextRequest } from "next/server"
import { describe, expect, it } from "vitest"
import { proxy } from "./proxy"

// These lock the cross-host routing contract that makes BYO custom domains work:
// a verified custom domain must serve the same / storefront + /admin ops
// admin as a tenant subdomain, while the platform-only surfaces (/account,
// /platform-admin) must NOT exist there. The backend resolves the tenant from
// the Host for both / and /admin, so the proxy must not block either.

const SESSION_COOKIE = "better-auth.session_token=test-session" // dev (http) cookie name

function request(host: string, pathname: string, opts: { session?: boolean } = {}): NextRequest {
  const headers: Record<string, string> = { host }
  if (opts.session) headers.cookie = SESSION_COOKIE
  return new NextRequest(`http://${host}${pathname}`, { headers })
}

function redirectTarget(res: Response): string | null {
  const loc = res.headers.get("location")
  return loc ? new URL(loc).pathname : null
}

async function run(host: string, pathname: string, opts?: { session?: boolean }) {
  return proxy(request(host, pathname, opts))
}

describe("proxy — custom domain (BYO)", () => {
  const host = "food.myevent.ch"

  it("serves / (public storefront)", async () => {
    const res = await run(host, "/")
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("serves /admin with a session", async () => {
    const res = await run(host, "/admin", { session: true })
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("redirects /admin without a session to /login", async () => {
    const res = await run(host, "/admin")
    expect(redirectTarget(res)).toBe("/login")
  })

  it("redirects /account to / (platform-only surface)", async () => {
    const res = await run(host, "/account", { session: true })
    expect(redirectTarget(res)).toBe("/")
  })

  it("redirects /t/<tenant> to / (platform-only hub)", async () => {
    const res = await run(host, "/t/incil", { session: true })
    expect(redirectTarget(res)).toBe("/")
  })

  it("redirects /platform-admin to / (platform-only surface)", async () => {
    const res = await run(host, "/platform-admin", { session: true })
    expect(redirectTarget(res)).toBe("/")
  })
})

describe("proxy — tenant subdomain", () => {
  const host = "myevent.comanda.leys.ch"

  it("serves /", async () => {
    const res = await run(host, "/")
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("serves /admin with a session", async () => {
    const res = await run(host, "/admin", { session: true })
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("redirects /account to / (platform-only surface)", async () => {
    const res = await run(host, "/account", { session: true })
    expect(redirectTarget(res)).toBe("/")
  })
})

// Regression: the session pre-check accepts either cookie name regardless of
// x-forwarded-proto (the /admin → /login → / loop).
describe("proxy — session cookie name is proto-independent", () => {
  const host = "food.myevent.ch"

  function reqWith(proto: string, cookie: string, pathname = "/admin"): NextRequest {
    return new NextRequest(`http://${host}${pathname}`, {
      headers: { host, "x-forwarded-proto": proto, "cf-visitor": '{"scheme":"https"}', cookie },
    })
  }

  it("serves /admin over https with the legacy non-__Secure- cookie", async () => {
    const res = await proxy(reqWith("https", "better-auth.session_token=s"))
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("serves /admin over http with the __Secure- cookie", async () => {
    const res = await proxy(reqWith("http", "__Secure-better-auth.session_token=s"))
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("still bounces /admin to /login when no session cookie is present", async () => {
    const res = await proxy(reqWith("https", "other=1"))
    expect(redirectTarget(res)).toBe("/login")
  })
})

// The upgrade must fire only on positive evidence that the *browser* used http;
// anything else loops on a custom domain whose edge forwards plain http.
describe("proxy — https upgrade", () => {
  const host = "food.myevent.ch"

  function req(pathname: string, headers: Record<string, string>): NextRequest {
    return new NextRequest(`http://${host}${pathname}`, { headers: { host, ...headers } })
  }

  it("upgrades a plain http visitor (no proxy in front)", async () => {
    const res = await proxy(req("/menu?x=1", { "x-forwarded-proto": "http" }))
    expect(res.status).toBe(308)
    expect(res.headers.get("location")).toBe(`https://${host}/menu?x=1`)
  })

  it("upgrades before the /api short-circuit", async () => {
    const res = await proxy(req("/api/orders", { "x-forwarded-proto": "http" }))
    expect(res.status).toBe(308)
    expect(res.headers.get("location")).toBe(`https://${host}/api/orders`)
  })

  it("upgrades when Cloudflare reports an http visitor", async () => {
    const res = await proxy(req("/", { "cf-visitor": '{"scheme":"http"}', "x-forwarded-proto": "https" }))
    expect(res.status).toBe(308)
  })

  it("serves an https visitor whose edge→origin hop is http (customer Flexible SSL)", async () => {
    const res = await proxy(req("/", { "cf-visitor": '{"scheme":"https"}', "x-forwarded-proto": "http" }))
    expect(res.status).toBe(200)
    expect(res.headers.get("strict-transport-security")).toBe("max-age=15552000")
  })

  it("never redirects when Cloudflare fronted the request but cf-visitor is unreadable", async () => {
    const res = await proxy(req("/", { "cf-ray": "abc-ZRH", "x-forwarded-proto": "http" }))
    expect(res.status).toBe(200)
  })

  it("never redirects when the scheme is unknown", async () => {
    const res = await proxy(req("/", {}))
    expect(res.status).toBe(200)
  })

  it("gives up after one attempt so an unknown TLS terminator can't loop", async () => {
    const res = await proxy(req("/", { "x-forwarded-proto": "http", cookie: "comanda-https-upgrade=1" }))
    expect(res.status).toBe(200)
  })

  it("sets the loop-breaker cookie on the redirect", async () => {
    const res = await proxy(req("/", { "x-forwarded-proto": "http" }))
    expect(res.headers.get("set-cookie")).toContain("comanda-https-upgrade=1")
  })

  it("leaves ACME HTTP-01 challenges on http", async () => {
    const res = await proxy(req("/.well-known/acme-challenge/tok", { "x-forwarded-proto": "http" }))
    expect(res.status).toBe(200)
  })

  it("omits HSTS from a cleartext response", async () => {
    const res = await proxy(req("/", { "cf-ray": "abc-ZRH", "x-forwarded-proto": "http" }))
    expect(res.headers.get("strict-transport-security")).toBeNull()
  })
})

describe("proxy — platform host", () => {
  const host = "comanda.leys.ch"

  it("serves /account with a session", async () => {
    const res = await run(host, "/account", { session: true })
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("serves /platform-admin with a session", async () => {
    const res = await run(host, "/platform-admin", { session: true })
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("serves /t/<tenant> with a session", async () => {
    const res = await run(host, "/t/incil", { session: true })
    expect(redirectTarget(res)).toBeNull()
    expect(res.status).toBe(200)
  })

  it("redirects /t/<tenant> without a session to /login", async () => {
    const res = await run(host, "/t/incil")
    expect(redirectTarget(res)).toBe("/login")
  })

  it("redirects /account without a session to /login", async () => {
    const res = await run(host, "/account")
    expect(redirectTarget(res)).toBe("/login")
  })
})
