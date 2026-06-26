import { betterAuth } from "better-auth"
import { emailOTP, organization } from "better-auth/plugins"
import { Pool } from "pg"
import { readFileSync } from "node:fs"
import { getAppEnv } from "./env"
import { normalizeHost, parseTenantHost } from "./tenant"

// Scaleway managed PG presents a private per-instance CA that isn't in Node's
// system roots. The DSN is sslmode=verify-full with no sslrootcert, so pin the
// mounted CA onto the pool (mirrors the backend's pgx pinning); without it the
// TLS handshake fails with DEPTH_ZERO_SELF_SIGNED_CERT. Unset locally
// (sslmode=disable).
function createAuthPool(): Pool {
  const connectionString = process.env.DATABASE_URL
  const caPath = process.env.DATABASE_CA_PATH
  if (caPath) {
    return new Pool({ connectionString, ssl: { ca: readFileSync(caPath, "utf8") } })
  }
  return new Pool({ connectionString })
}

// Better Auth owns this pool; we also use it to resolve the active organization
// on session creation (see databaseHooks below). DATABASE_URL is the privileged
// DSN (it owns the auth tables), so these reads bypass RLS — no tenant GUC is
// bound here.
const authPool = createAuthPool()

// Resolve which organization a newly-created session should activate, based on
// the host the user authenticated against. org == tenant, so a vendor signing in
// on their tenant host (subdomain or verified custom domain) must land with that
// tenant's org active — otherwise get-active-member returns NO_ACTIVE_ORGANIZATION
// and the admin storefront is unreachable. Every branch is membership-gated: a
// non-member (e.g. an end customer ordering on the tenant storefront) resolves to
// null and stays org-less, which is correct.
async function resolveActiveOrgForHost(userId: string, host: string | undefined): Promise<string | null> {
  const tenant = parseTenantHost(host || "")

  if (tenant.type === "tenant-subdomain") {
    const { rows } = await authPool.query<{ organization_id: string }>(
      `SELECT t.organization_id
         FROM tenant t
         JOIN "member" m ON m.organization_id = t.organization_id
        WHERE t.slug = $1 AND m.user_id = $2
        LIMIT 1`,
      [tenant.slug, userId]
    )
    return rows[0]?.organization_id ?? null
  }

  if (tenant.type === "custom-domain") {
    const { rows } = await authPool.query<{ organization_id: string }>(
      `SELECT t.organization_id
         FROM tenant_domain d
         JOIN tenant t ON t.id = d.tenant_id
         JOIN "member" m ON m.organization_id = t.organization_id
        WHERE d.hostname = $1 AND d.verified_at IS NOT NULL AND m.user_id = $2
        LIMIT 1`,
      [normalizeHost(host), userId]
    )
    return rows[0]?.organization_id ?? null
  }

  // Platform host (the vendor account hub): no tenant in the host. Fall back to
  // the user's sole membership so the account hub works after a fresh sign-in;
  // when the user belongs to several orgs it's ambiguous, so leave it unset.
  const { rows } = await authPool.query<{ organization_id: string }>(
    `SELECT organization_id FROM "member" WHERE user_id = $1 LIMIT 2`,
    [userId]
  )
  return rows.length === 1 ? (rows[0]!.organization_id ?? null) : null
}

// Module-level cache for the trusted-origins fetch. Refreshes every 60 s so a
// backend blip never permanently breaks auth, and a newly-verified domain shows
// up without a restart.
let _customOriginsCache: { hostnames: string[]; fetchedAt: number } | null = null
const CUSTOM_ORIGINS_TTL_MS = 60_000

async function fetchCustomOrigins(): Promise<string[]> {
  const now = Date.now()
  if (_customOriginsCache && now - _customOriginsCache.fetchedAt < CUSTOM_ORIGINS_TTL_MS) {
    return _customOriginsCache.hostnames
  }

  const backendUrl = process.env.BACKEND_INTERNAL_URL
  const token = process.env.INTERNAL_API_TOKEN
  if (!backendUrl || !token) return []

  const res = await fetch(`${backendUrl}/internal/trusted-origins`, {
    headers: { Authorization: `Bearer ${token}` },
    // Use a short timeout so a slow backend doesn't stall auth handling.
    signal: AbortSignal.timeout(3_000),
  })
  if (!res.ok) return []
  const data = (await res.json()) as { hostnames?: string[] }
  const hostnames = Array.isArray(data.hostnames) ? data.hostnames : []
  _customOriginsCache = { hostnames, fetchedAt: now }
  return hostnames
}

// OTP types supported by the backend
type OTPType = "sign-in" | "email-verification" | "forget-password"

// Notify Go backend to look up the OTP from the database and send the email.
// host is the tenant's request host, forwarded so the backend's host-based
// resolver scopes the lookup + sender to the right tenant — this server-to-server
// call otherwise carries only the internal service host (→ 404 "unknown host").
async function sendOTPEmailViaBackend(email: string, type: OTPType, host?: string): Promise<void> {
  const backendUrl = process.env.BACKEND_INTERNAL_URL || process.env.NEXT_PUBLIC_API_BASE_URL
  if (!backendUrl) {
    throw new Error("BACKEND_INTERNAL_URL or NEXT_PUBLIC_API_BASE_URL is required")
  }

  const headers: Record<string, string> = { "Content-Type": "application/json" }
  if (host) headers["X-Forwarded-Host"] = host

  const response = await fetch(`${backendUrl}/v1/auth/otp-email`, {
    method: "POST",
    headers,
    body: JSON.stringify({ email, type }),
  })

  if (!response.ok) {
    const errorBody = await response.text()
    throw new Error(`Failed to send OTP email: status=${response.status} body=${errorBody}`)
  }
}

export const auth = betterAuth({
  database: authPool,

  secret: process.env.BETTER_AUTH_SECRET,

  // org == tenant. Better Auth never sets activeOrganizationId on its own, so a
  // fresh sign-in would land org-less and the org plugin's get-active-member
  // would 400 with NO_ACTIVE_ORGANIZATION. Resolve it from the auth host here so
  // every new session starts bound to the right tenant's org (membership-gated).
  databaseHooks: {
    session: {
      create: {
        before: async (session, ctx) => {
          const h = ctx as { request?: Request; headers?: Headers } | null
          const headers = h?.request?.headers ?? h?.headers
          const host =
            headers?.get("x-comanda-host") || headers?.get("x-forwarded-host") || headers?.get("host") || undefined
          const activeOrganizationId = await resolveActiveOrgForHost(session.userId, host)
          if (activeOrganizationId) {
            return { data: { ...session, activeOrganizationId } }
          }
        },
      },
    },
  },

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
      sendVerificationOTP: async ({ email, type }, ctx) => {
        // OTP is not sent over the network — the backend reads it directly from
        // the verification table by email (identifier). We forward the tenant
        // host from the originating request so the backend can resolve the tenant.
        const reqHeaders = ctx as { request?: Request; headers?: Headers } | undefined
        const h = reqHeaders?.request?.headers ?? reqHeaders?.headers
        const host = h?.get("x-comanda-host") || h?.get("x-forwarded-host") || h?.get("host") || undefined
        await sendOTPEmailViaBackend(email, type as OTPType, host)
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

  // We deliberately never push the inbound request origin blindly — that would
  // bypass Better Auth's CSRF/origin protection; the backend's 404-on-unknown-host
  // is the real isolation boundary. Note: better-auth matches host-only globs
  // (no "://") against the host *including the port*, so dev needs scheme+port
  // globs while prod (no port) can use bare host globs.
  trustedOrigins: async (_request?: Request): Promise<string[]> => {
    const appEnv = getAppEnv()
    const isDev = appEnv === "dev"

    const origins: string[] = ["*.comanda.leys.ch"]

    // *.comanda.leys.ch is single-label and won't match the nested staging apex.
    if (appEnv === "staging") {
      origins.push("*.staging.comanda.leys.ch")
    }

    const platformHosts = (process.env.PLATFORM_HOSTS || "")
      .split(",")
      .map((h) => h.trim().toLowerCase())
      .filter(Boolean)
    for (const h of platformHosts) {
      origins.push(`https://${h}`)
      if (isDev) origins.push(`http://${h}`, `http://${h}:*`)
    }

    if (isDev) {
      origins.push("http://localhost:*", "http://127.0.0.1:*", "http://*.localhost:*")
    }

    const appUrl = process.env.APP_URL ?? process.env.NEXT_PUBLIC_APP_URL
    if (appUrl) {
      origins.push(appUrl)
    }

    // Append verified custom-domain origins. Falls back silently if the token
    // is unset or the internal endpoint is unreachable (fail-open for origins,
    // not fail-closed — the real tenant isolation boundary is the backend RLS).
    try {
      const customHostnames = await fetchCustomOrigins()
      for (const h of customHostnames) {
        origins.push(`https://${h}`)
      }
    } catch {
      // Ignore — the static origins above are still returned.
    }

    return origins
  },

  advanced: {
    // Deterministic __Secure- cookie name outside dev (all non-dev traffic is HTTPS).
    useSecureCookies: getAppEnv() !== "dev",
    // Do NOT set crossSubDomainCookies — it would share the session cookie across
    // every *.comanda.leys.ch, a tenant-session-leakage vector.
  },
})

// Export type for use in other server files
export type Auth = typeof auth

// Export handler for API routes
export const authHandler = auth.handler

// Export API for server-side session access
export const authApi = auth.api
