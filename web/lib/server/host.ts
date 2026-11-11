import { headers } from "next/headers"
import { normalizeHost, parseTenantHost } from "@/lib/tenant"

/**
 * Returns the canonical public host for the current server-side request.
 *
 * Reads x-comanda-host (pinned by proxy.ts from the real public hostname),
 * then x-forwarded-host, then host. Returns an empty string if none present.
 *
 * Use this in Server Components and Server Actions that call the backend
 * directly (via lib/server/api.ts) rather than through the /api proxy.
 */
export async function getRequestHost(): Promise<string> {
  const hdrs = await headers()
  return normalizeHost(hdrs.get("x-comanda-host") || hdrs.get("x-forwarded-host") || hdrs.get("host"))
}

export async function isPlatformRequest(): Promise<boolean> {
  return parseTenantHost(await getRequestHost()).type === "platform"
}
