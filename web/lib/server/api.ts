/**
 * Server-side API fetch wrapper.
 *
 * When Next.js Server Components or Server Actions call the backend directly
 * (bypassing the /api proxy route), they must carry X-Forwarded-Host so the
 * backend can resolve the tenant. This wrapper reads the current request host
 * via getRequestHost() and injects it automatically.
 *
 * Usage:
 *   import { serverApiRequest } from "@/lib/server/api"
 *   const data = await serverApiRequest<MenuResponse>("/v1/menu")
 */
import { apiRequest } from "@/lib/api"
import { getRequestHost } from "./host"

export async function serverApiRequest<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const host = await getRequestHost()
  const extraHeaders: Record<string, string> = {}
  if (host) {
    extraHeaders["X-Forwarded-Host"] = host
  }
  return apiRequest<T>(endpoint, {
    ...options,
    headers: {
      ...extraHeaders,
      ...(options.headers as Record<string, string> | undefined),
    },
  })
}
