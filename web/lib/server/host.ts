import { headers } from "next/headers"

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
  const xch = hdrs.get("x-comanda-host") || ""
  if (xch) return xch
  const xfh = hdrs.get("x-forwarded-host") || ""
  const raw = xfh || hdrs.get("host") || ""
  return raw.split(",")[0]!.trim().split(":")[0]!.toLowerCase()
}
