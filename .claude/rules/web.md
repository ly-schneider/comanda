---
paths:
  - "web/**"
---

# Web

- `proxy.ts` is the Next.js 16 middleware. Don't rename it to `middleware.ts`.
- Server Components that call the backend directly must forward the request host with `getRequestHost()` from `lib/server/host.ts`, or tenant resolution fails on staging. Local development hides this because `localhost` resolves to a tenant.
- `isPlatformRequest()` tells the platform host (marketing site, organiser hub, platform admin) apart from tenant shops.
- Every surface is light. Don't add dark mode or `prefers-color-scheme` styles.
- Use `pnpm`. Run `pnpm typecheck`, `pnpm lint` and `pnpm test` before committing.
