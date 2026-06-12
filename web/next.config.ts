import withBundleAnalyzer from "@next/bundle-analyzer"
import { withSentryConfig } from "@sentry/nextjs"
import { type NextConfig } from "next"

if (!process.env.APP_VERSION) {
  process.env.APP_VERSION = "dev"
}

if (!process.env.NEXT_PUBLIC_APP_VERSION) {
  process.env.NEXT_PUBLIC_APP_VERSION = process.env.APP_VERSION
}

const config: NextConfig = {
  devIndicators: false,
  reactStrictMode: true,
  output: "standalone",
  outputFileTracingExcludes: {
    "/**": [
      "node_modules/@swc/core/**",
      "node_modules/webpack/**",
      "node_modules/@babel/core/**",
      "node_modules/typescript/**",
      "node_modules/esbuild/**",
      "node_modules/@esbuild/**",
    ],
  },
  logging: {
    fetches: {
      fullUrl: true,
    },
  },
  async headers() {
    return [
      {
        source: "/station-sw.js",
        headers: [
          { key: "Service-Worker-Allowed", value: "/station" },
          { key: "Cache-Control", value: "no-cache" },
        ],
      },
    ]
  },
  images: {
    formats: ["image/avif", "image/webp"],
    minimumCacheTTL: 2592000,
    remotePatterns: [
      { protocol: "http", hostname: "localhost", port: "8080", pathname: "/**" },
      { protocol: "http", hostname: "127.0.0.1", port: "8080", pathname: "/**" },
      { protocol: "https", hostname: "*.leys.ch", pathname: "/**" },
      { protocol: "https", hostname: "*.comanda.leys.ch", pathname: "/**" },
      // Public object-storage host (Scaleway CDN domain) — env-driven, derived
      // from STORAGE_PUBLIC_URL. Falls back to the leys.ch CDN wildcard.
      ...(process.env.STORAGE_PUBLIC_URL
        ? [
            {
              protocol: new URL(process.env.STORAGE_PUBLIC_URL).protocol.replace(":", "") as "https" | "http",
              hostname: new URL(process.env.STORAGE_PUBLIC_URL).hostname,
              pathname: "/**",
            },
          ]
        : [{ protocol: "https" as const, hostname: "**.leys.ch", pathname: "/**" }]),
      // Local MinIO (S3 API) for dev object storage.
      { protocol: "http", hostname: "localhost", port: "9000", pathname: "/**" },
      { protocol: "http", hostname: "127.0.0.1", port: "9000", pathname: "/**" },
    ],
    qualities: [90],
  },
}

const sentryWrapped = withSentryConfig(config, {
  sourcemaps: {
    disable: !process.env.CI,
  },
  silent: !process.env.CI,
  telemetry: false,
})

const analyze = process.env.ANALYZE === "true"
export default analyze ? withBundleAnalyzer({ enabled: analyze })(sentryWrapped) : sentryWrapped
