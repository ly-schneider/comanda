import { isDevEnv } from "@/lib/env"

export type CookieNames = { secure: boolean; csrfName: string }

// Cookie naming follows the browser-facing scheme (https everywhere except dev),
// not x-forwarded-proto — a custom domain's edge→origin hop is http, which would
// otherwise pick the wrong name and break the CSRF double-submit check. Mirrors
// the writer in proxy.ts and the better-auth session cookie's useSecureCookies.
export function resolveCookieNames(): CookieNames {
  const secure = !isDevEnv()
  return {
    secure,
    csrfName: secure ? "__Host-csrf" : "csrf",
  }
}
