import { toNextJsHandler } from "better-auth/next-js"
import { authHandler } from "@/lib/auth"
import { getAppEnv } from "@/lib/env"

const handlers = toNextJsHandler(authHandler)

// Behind the ingress the request hits Next.js over plain HTTP on an internal
// host, so `request.url` carries neither the public hostname nor https. The
// oauth-proxy plugin derives the originating tenant origin from `request.url`
// (resolveCurrentURL / checkSkipProxy), so rewrite it to the public origin —
// scheme from the env (https everywhere except dev), host from the
// forwarded/Host header — before better-auth sees it. The path/query and the
// browser-set Origin header are preserved, so routing and CSRF are unaffected.
async function withPublicOrigin(req: Request): Promise<Request> {
  const host = (req.headers.get("x-forwarded-host") || req.headers.get("host") || "")
    .split(",")[0]
    ?.trim()
    .toLowerCase()
  if (!host) return req

  const src = new URL(req.url)
  const proto = getAppEnv() === "dev" ? "http" : "https"
  const target = new URL(src.pathname + src.search, `${proto}://${host}`)
  if (target.href === src.href) return req

  const hasBody = req.method !== "GET" && req.method !== "HEAD"
  const body = hasBody ? await req.arrayBuffer() : undefined
  return new Request(target, { method: req.method, headers: req.headers, body })
}

export async function POST(req: Request): Promise<Response> {
  return handlers.POST(await withPublicOrigin(req))
}

export async function GET(req: Request): Promise<Response> {
  return handlers.GET(await withPublicOrigin(req))
}
