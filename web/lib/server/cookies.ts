import { isDevEnv } from "@/lib/env"

export type CookieNames = { secure: boolean; csrfName: string }

// Key off env (browser scheme), not x-forwarded-proto; custom-domain edge→origin
// is http. Must match the writer in proxy.ts.
export function resolveCookieNames(): CookieNames {
  const secure = !isDevEnv()
  return {
    secure,
    csrfName: secure ? "__Host-csrf" : "csrf",
  }
}
