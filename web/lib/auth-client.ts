"use client"

import { adminClient, emailOTPClient, organizationClient } from "better-auth/client/plugins"
import { createAuthClient } from "better-auth/react"
import { ac, adminRole, customerRole } from "./auth/permissions"

export const authClient = createAuthClient({
  // On the server (SSR), use the configured app URL.
  // In the browser, leave undefined so Better Auth uses window.location.origin —
  // this is correct for multi-host: each tenant host makes auth requests to
  // its own origin rather than a hardcoded single-tenant URL.
  baseURL: typeof window === "undefined" ? process.env.NEXT_PUBLIC_APP_URL : undefined,
  plugins: [
    emailOTPClient(),
    adminClient({
      ac,
      roles: {
        customer: customerRole,
        admin: adminRole,
      },
    }),
    organizationClient(),
  ],
})

// Export commonly used methods for convenience
export const { signIn, signOut, useSession } = authClient

// Plugin-specific exports
export const emailOtp = authClient.emailOtp
export const adminApi = authClient.admin
export const orgApi = authClient.organization
