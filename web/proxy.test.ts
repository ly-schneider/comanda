import { NextRequest } from "next/server"
import { describe, expect, it } from "vitest"
import { proxy } from "./proxy"

// These lock the cross-host routing contract that makes BYO custom domains work:
// a verified custom domain must serve the same /food storefront + /admin ops
// admin as a tenant subdomain, while the platform-only surfaces (/account,
// /platform-admin) must NOT exist there. The backend resolves the tenant from
// the Host for both /food and /admin, so the proxy must not block either.

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

  it("serves /food (public storefront)", async () => {
    const res = await run(host, "/food")
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

  it("serves /food", async () => {
    const res = await run(host, "/food")
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
// x-forwarded-proto (the /admin → /login → /food loop).
describe("proxy — session cookie name is proto-independent", () => {
  const host = "food.myevent.ch"

  function reqWith(proto: string, cookie: string, pathname = "/admin"): NextRequest {
    return new NextRequest(`http://${host}${pathname}`, {
      headers: { host, "x-forwarded-proto": proto, cookie },
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
