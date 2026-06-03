import { betterAuth } from "better-auth"
import { admin, emailOTP, organization } from "better-auth/plugins"
import { Pool } from "pg"
import { ac, adminRole, customerRole } from "./auth/permissions"

// OTP types supported by the backend
type OTPType = "sign-in" | "email-verification" | "forget-password"

// Notify Go backend to look up the OTP from the database and send the email
async function sendOTPEmailViaBackend(email: string, type: OTPType): Promise<void> {
  const backendUrl = process.env.BACKEND_INTERNAL_URL || process.env.NEXT_PUBLIC_API_BASE_URL
  if (!backendUrl) {
    throw new Error("BACKEND_INTERNAL_URL or NEXT_PUBLIC_API_BASE_URL is required")
  }

  const response = await fetch(`${backendUrl}/v1/auth/otp-email`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ email, type }),
  })

  if (!response.ok) {
    const errorBody = await response.text()
    throw new Error(`Failed to send OTP email: status=${response.status} body=${errorBody}`)
  }
}

export const auth = betterAuth({
  database: new Pool({
    connectionString: process.env.DATABASE_URL,
  }),

  secret: process.env.BETTER_AUTH_SECRET,

  // Disable email/password - using OTP instead
  emailAndPassword: { enabled: false },

  // Map core table fields to snake_case (PostgreSQL convention)
  user: {
    fields: {
      emailVerified: "email_verified",
      createdAt: "created_at",
      updatedAt: "updated_at",
    },
    additionalFields: {
      role: {
        type: "string",
        defaultValue: "customer",
        input: false, // Don't allow setting via signup
      },
      isPlatformAdmin: {
        type: "boolean",
        defaultValue: false,
        input: false, // Admin-set out of band; never writable via the auth API
        fieldName: "is_platform_admin",
      },
    },
  },
  session: {
    expiresIn: 60 * 60 * 24 * 90, // 90 days
    updateAge: 60 * 60 * 24, // refresh daily on activity
    fields: {
      userId: "user_id",
      expiresAt: "expires_at",
      ipAddress: "ip_address",
      userAgent: "user_agent",
      createdAt: "created_at",
      updatedAt: "updated_at",
    },
  },
  account: {
    fields: {
      userId: "user_id",
      accountId: "account_id",
      providerId: "provider_id",
      accessToken: "access_token",
      refreshToken: "refresh_token",
      accessTokenExpiresAt: "access_token_expires_at",
      refreshTokenExpiresAt: "refresh_token_expires_at",
      idToken: "id_token",
      createdAt: "created_at",
      updatedAt: "updated_at",
    },
  },
  verification: {
    fields: {
      expiresAt: "expires_at",
      createdAt: "created_at",
      updatedAt: "updated_at",
    },
  },

  socialProviders: {
    google: {
      clientId: process.env.GOOGLE_CLIENT_ID!,
      clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
    },
  },

  plugins: [
    // Email OTP (passwordless sign-in)
    emailOTP({
      otpLength: 6,
      expiresIn: 300, // 5 minutes
      sendVerificationOTP: async ({ email, type }: { email: string; otp: string; type: string }) => {
        // OTP is not sent over the network — the backend reads it
        // directly from the verification table by email (identifier)
        await sendOTPEmailViaBackend(email, type as OTPType)
      },
    }),

    // Admin plugin for managing users and invites
    admin({
      ac,
      roles: {
        customer: customerRole,
        admin: adminRole,
      },
      defaultRole: "customer",
      adminRoles: ["admin"],
      schema: {
        user: {
          fields: {
            banReason: "ban_reason",
            banExpires: "ban_expires",
          },
        },
        session: {
          fields: {
            impersonatedBy: "impersonated_by",
          },
        },
      },
    }),

    // Organization plugin: org = tenant, members = vendor staff/admins.
    // End-customers are per-tenant users (scoped by signup host) and are NOT
    // org members. Org creation is platform-provisioned only (P7).
    organization({
      allowUserToCreateOrganization: false,
      schema: {
        session: {
          fields: {
            activeOrganizationId: "active_organization_id",
          },
        },
        organization: {
          fields: {
            createdAt: "created_at",
          },
        },
        member: {
          fields: {
            organizationId: "organization_id",
            userId: "user_id",
            createdAt: "created_at",
          },
        },
        invitation: {
          fields: {
            organizationId: "organization_id",
            inviterId: "inviter_id",
            expiresAt: "expires_at",
            createdAt: "created_at",
          },
        },
      },
    }),
  ],

  // Dynamic trustedOrigins: trust all comanda.leys.ch subdomains (prod +
  // staging), *.localhost for dev, and any configured platform hosts.
  //
  // We deliberately do NOT push the inbound request origin blindly — that
  // would bypass Better Auth's own CSRF/origin protection. The backend's
  // 404-on-unknown-host is the real isolation boundary.
  //
  // TODO(P6): once verified custom domains are provisioned, fetch the
  // verified-domain list from the backend and include those origins here.
  trustedOrigins: async (_request?: Request): Promise<string[]> => {
    const origins: string[] = [
      // Wildcard patterns — Better Auth's matchesOriginPattern supports * globs.
      "*.comanda.leys.ch",
      "*.beta.comanda.leys.ch",
      "*.localhost",
      // Loopback http for dev
      "http://localhost",
      "http://127.0.0.1",
    ]

    // Extra platform-level hosts supplied via env (e.g. comanda.leys.ch itself).
    const platformHosts = (process.env.PLATFORM_HOSTS || "")
      .split(",")
      .map((h) => h.trim())
      .filter(Boolean)
    origins.push(...platformHosts)

    // Single-host dev fallback (NEXT_PUBLIC_APP_URL set in .env.local).
    if (process.env.NEXT_PUBLIC_APP_URL) {
      origins.push(process.env.NEXT_PUBLIC_APP_URL)
    }

    return origins
  },

  // Cookie security: do NOT set crossSubDomainCookies. Without a domain
  // attribute, browsers scope each session cookie to the exact issuing host.
  // Setting crossSubDomainCookies would share the session cookie across all
  // *.comanda.leys.ch — a direct tenant-session-leakage vector.
})

// Export type for use in other server files
export type Auth = typeof auth

// Export handler for API routes
export const authHandler = auth.handler

// Export API for server-side session access
export const authApi = auth.api
