import * as Sentry from "@sentry/nextjs"
import { toSentryEnvironment } from "@/lib/env"
import { appEnvFromHost } from "@/lib/tenant"

const dsn = process.env.NEXT_PUBLIC_SENTRY_DSN

if (dsn) {
  const appEnv = typeof window !== "undefined" ? appEnvFromHost(window.location.hostname) : "prod"

  Sentry.init({
    dsn,
    environment: toSentryEnvironment(appEnv),
    release: process.env.NEXT_PUBLIC_APP_VERSION,
    tracesSampleRate: 0.2,
    enableLogs: true,
    integrations: [Sentry.browserTracingIntegration(), Sentry.consoleLoggingIntegration({ levels: ["warn", "error"] })],
  })
}
