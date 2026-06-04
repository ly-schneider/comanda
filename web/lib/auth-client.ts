"use client"

import { adminClient, emailOTPClient, organizationClient } from "better-auth/client/plugins"
import { createAuthClient } from "better-auth/react"
import { ac, adminRole, customerRole } from "./auth/permissions"

export const authClient = createAuthClient({
  // Browser: undefined so each tenant host authenticates against its own origin.
  // SSR: the runtime APP_URL.
  baseURL: typeof window === "undefined" ? (process.env.APP_URL ?? process.env.NEXT_PUBLIC_APP_URL) : undefined,
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
