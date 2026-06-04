import * as Sentry from "@sentry/nextjs"
import { getAppEnv, toSentryEnvironment } from "@/lib/env"

const dsn = process.env.NEXT_PUBLIC_SENTRY_DSN

if (dsn) {
  Sentry.init({
    dsn,
    environment: toSentryEnvironment(getAppEnv()),
    release: process.env.APP_VERSION,
    tracesSampleRate: 0.2,
    enableLogs: true,
    integrations: [Sentry.consoleLoggingIntegration({ levels: ["warn", "error"] })],
  })
}
