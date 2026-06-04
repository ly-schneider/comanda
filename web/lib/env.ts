// Server-only: reads the runtime APP_ENV container var. Never call from
// statically-prerendered components (bakes at build time) — client code must
// derive the env from the hostname via appEnvFromHost() in lib/tenant.ts.

export type AppEnv = "dev" | "staging" | "prod"

export function getAppEnv(): AppEnv {
  const raw = (process.env.APP_ENV || "").trim().toLowerCase()
  if (raw === "dev" || raw === "staging" || raw === "prod") return raw
  return process.env.NODE_ENV === "development" ? "dev" : "prod"
}

export const isDevEnv = (): boolean => getAppEnv() === "dev"
export const isStagingEnv = (): boolean => getAppEnv() === "staging"
export const isProdEnv = (): boolean => getAppEnv() === "prod"

export function toSentryEnvironment(env: AppEnv): "production" | "staging" | "development" {
  if (env === "staging") return "staging"
  if (env === "dev") return "development"
  return "production"
}
