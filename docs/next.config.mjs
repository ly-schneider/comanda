import { createMDX } from "fumadocs-mdx/next";

if (!process.env.APP_VERSION) {
  process.env.APP_VERSION = "dev";
}

const withMDX = createMDX();

/** @type {import('next').NextConfig} */
const config = {
  output: "standalone",
  reactStrictMode: true,
  devIndicators: false,
  // File tracing only sees the CJS helpers it can resolve statically, but the
  // compiled server asks for @swc/helpers/esm/* at runtime — so a standalone
  // build boots straight into MODULE_NOT_FOUND. Pull the whole package in.
  outputFileTracingIncludes: {
    "/**": ["./node_modules/.pnpm/@swc+helpers@*/node_modules/@swc/helpers/**"],
  },
};

export default withMDX(config);
