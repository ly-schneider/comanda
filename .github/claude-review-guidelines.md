# Claude code-review guidelines

Canonical, human-editable rules the automated reviewer applies to every PR. The
workflow (`.github/workflows/claude-code-review.yml`) appends a condensed core of
this to the reviewer's system prompt and instructs it to read this file in full.
**Edit here first**; keep the workflow's inline core roughly in sync.

These rules encode the real, repeated conventions of this repo so changes don't
drift from them. They are derived from the code itself, not aspiration — cite the
file/symbol when you flag a violation, and explain the _why_, not just the _what_.

## How to review (stance)

- **Scope:** flag (1) tenancy/RLS/auth/crypto/payment issues, (2) convention &
  comment-policy drift, (3) generated-code & migration violations, and (4)
  performance/efficiency — on top of the default correctness and
  reuse/simplification review.
- **Enforce forward, grandfather existing.** Flag _new_ violations. Don't demand
  edits to already-applied migrations (they can't be renamed) or to pre-existing
  deviations unless the PR is already touching them.
- **Don't extend legacy, don't nag about it.** Known dead/scaffolding code (see
  "Legacy" below) is fine as-is — but flag any _new_ code that builds on or
  imitates those dead patterns.
- **Tests are required for all new logic**, not just the security-critical
  surface. Security/tenancy-critical changes additionally need the matching RLS
  coverage / adversarial unit tests (see "Testing").
- **Severity:** treat any weakening of the tenant-isolation boundary or secret
  handling as **blocking**. Convention/comment/perf items are normal findings.

## Stack

- **Backend** — Go + Ent + Chi, Clean Architecture, Uber FX DI, Postgres.
  Row-Level Security (RLS) is the tenant-isolation boundary.
- **Web** — Next.js 16 / React 19, Better Auth (organization plugin), host-based
  tenant resolution. `proxy.ts` is the active middleware (not `middleware.ts`).
- **Android** — a single neutral Kotlin POS app (WebView shell over the web
  `/pos`); all branding is dynamic/runtime.
- **Infra** — Hetzner K3s via ArgoCD gitops; images pinned by commit SHA. No Azure.

---

## 1. Tenancy & security (blocking) — the isolation boundary

### Backend

- **Two Ent clients.** Request traffic uses the RLS-bound _app_ client; the
  _privileged_ client bypasses RLS and is only for migrations, cron, boot hooks,
  host→tenant resolution, and webhook tenant resolution. Flag any handler/service/
  repository wired to the privileged client outside those exceptions
  (`internal/database/postgres.go`, `internal/app/database.go`).
- **Tenant scope is set only by middleware**, via one pgx tx per request running
  `set_config('app.tenant_id', …, true)`; all request DB work runs inside that tx
  (`internal/middleware/tenant_tx.go`, `internal/http/router.go`). Flag a new
  route group registered without `Resolve`+`Tx`, or DB work that opens its own
  connection/tx without setting the GUC.
- **Repositories must go through `r.ec(ctx)`** (= `ClientFromContext`), never the
  bare `r.client` — the bare pool runs outside the request tx, so the GUC isn't
  set and RLS fails closed. Flag any repo method using `r.client.<Entity>`.
  (`product_repo.go` is a known pre-existing exception — grandfathered, but no new
  repo may copy it.)
- **Every tenant-scoped Ent schema must line up in three places:** embed
  `TenantMixin{}`, appear in `enttenant.scopedTypes`, and get an RLS policy in the
  migration. A schema with the mixin but missing from `scopedTypes` is treated as
  global = a leak. The coverage tests enforce this; the reviewer should too.
- **`enttenant.Register` is attached only to the app client**, never the
  privileged one (`internal/database/postgres.go`).
- **Platform-admin bypass is double-gated:** the verified `user.is_platform_admin`
  column flips the `app.is_platform_admin` GUC, and only the literals
  `'true'`/`'false'` are ever written (`internal/auth/session_middleware.go`).
  Flag setting that GUC from user input or outside `syncPlatformAdminGUC`.
- **Pre-tx tenant resolution** (host, webhook token) uses the privileged client to
  read the mapping, then opens its own RLS-bound tenant tx mirroring the
  middleware before touching tenant data (`tenant_tx.go`, `handlers_payment.go`).
- **Secrets at rest are AES-256-GCM** via `internal/crypto.Encryptor`; only
  ciphertext is stored and the encrypted columns are named `*Enc`. The app
  fails fast at boot without `PAYMENTS_MASTER_KEY`. Flag any credential column
  stored without encryption / the `_Enc` convention.
- **Per-tenant payments only via `payment.Factory.For(ctx)`**, which builds a
  single-tenant `payment.Provider` from the RLS-scoped, decrypted settings inside
  the tenant tx. Flag constructing a Payrexx client directly in a handler/service,
  or caching one provider across tenants.
- **Webhooks route per tenant** by opaque path token `{tenantToken}`, verified
  with that tenant's own HMAC secret (constant-time compare); wrong token → 404,
  bad/foreign signature → 401. Flag a webhook that skips verification, uses a
  global secret, or trusts the tenant from the body.
- **The QR token is a frozen cross-language wire contract:** Ed25519 over
  canonical JSON with fixed key order `v,t,o,i,l`; verify over the literal decoded
  bytes (never re-serialize); enforce version + tenant checks
  (`internal/qrsign`). Flag reordering payload fields, marshaling a map, bumping
  `Version` without a coordinated rollout, or re-serializing before verify.
- **Validate externally-supplied ids** with `nanoid.Valid` before using them in a
  query (headers, webhook reference ids, path params).

### Web

- **Tenant identity is always host-derived; never trust a client-supplied tenant
  id.** The `/api` proxy actively strips `X-Tenant-Id`; the backend resolves
  tenant from `X-Forwarded-Host` only (`app/api/[...path]/route.ts`, `lib/tenant.ts`).
  Flag any code that sets/forwards a tenant id or scopes by a query param.
- **Canonical host comes from `proxy.ts`** pinned as `x-comanda-host`, with
  precedence `x-comanda-host → x-forwarded-host → host` via `normalizeHost()`.
  Flag direct `headers.get("host")` for tenant logic.
- **`proxy.ts` IS the middleware and is string-only** — no DB, no async/fetch in
  tenant classification. Flag creating `middleware.ts`, or adding any
  `await`/`fetch`/DB call to `proxy()` / `parseTenantHost()`.
- **A valid QR signature is necessary but not sufficient** — the caller must
  re-check `payload.t === stationTenantId` (`lib/station/qr-verify.ts`,
  `redeem.ts`). Flag any consumer of `verifyQrToken` that acts on the payload
  without the tenant check.
- **No cross-subdomain cookies.** Don't add `crossSubDomainCookies`, a cookie
  `domain`, or push the raw inbound origin into `trustedOrigins` (`lib/auth.ts`) —
  it leaks sessions across tenants. `trustedOrigins` is an explicit host-glob
  allowlist.
- **Privilege fields are non-writable via the auth API** (`role`,
  `is_platform_admin` use `input: false`). Flag a new privilege field without it.
- **Server→backend calls must forward the tenant host.** Use `serverApiRequest` /
  `getBranding` (which inject `X-Forwarded-Host`); plain `apiRequest` on the
  server does **not**. Flag a server component calling the backend without host
  forwarding — without the host the backend can't resolve the tenant.
- **Mutating browser requests carry the double-submit CSRF token**; the proxy
  validates header-vs-cookie and has a fixed `csrfExempt` prefix list plus a
  Bearer bypass. A new cookie-authed mutating route generally needs the CSRF flow;
  device/Bearer routes go on the exempt list. The CSRF cookie name is
  environment-aware (`__Host-csrf` on https) and must stay consistent across
  `proxy.ts`, `lib/csrf.ts`, `lib/server/cookies.ts`.
- Host allowlists in three files must stay in sync when hosts change:
  `RESERVED_SLUGS`/`PLATFORM_BARE_HOSTS` (`lib/tenant.ts`), `trustedOrigins`
  (`lib/auth.ts`), and `remotePatterns` (`next.config.ts`).

---

## 2. Backend conventions

- **Strict layering:** api(handlers) → service → repository → Ent. Handlers never
  touch Ent/SQL directly; repositories never import `auth`/`config`/`net/http`
  (would form an import cycle — repos read tenant id from ctx). The webhook
  handler holding `privileged`+`appDB` is the documented exception.
- **FX wiring** lives in per-layer `app/New*()` bundles; `cmd/backend/main.go` is
  the single composition root. A new repo/service/handler is registered in the
  matching bundle. Services depend on repository _interfaces_, not concretes.
- **Error/response shape:** handlers respond via `response.WriteJSON` + the
  generated DTO, and errors via `writeError` / `writeEntError` over
  `generated.Error{code,message}`. Repos translate Ent/pg errors to sentinels
  (`ErrNotFound`, `ErrConflict`) via `translateError`; services wrap with
  `fmt.Errorf("…: %w", err)`. (The `internal/response` `ProblemDetails`/
  `WriteSuccess` path is legacy — see "Legacy"; don't use it in new handlers.)
- **IDs:** all PKs are 12-char nanoid via `nanoidPK()` (app-side, no DB default,
  alphabet excludes confusables). Flag `field.UUID`/auto-increment/custom PKs.
- **Context:** propagate `r.Context()` everywhere; `ctx` is the first arg of every
  service/repo method. No `context.Background()`/`TODO()` on the request path
  (the one legitimate exception — detached background work — must re-open its own
  tenant tx).
- **Context keys** are unexported typed keys with paired `ContextWith*`/
  `*FromContext` accessors; no bare-string keys or raw `ctx.Value` outside them.
- **ent↔DTO mapping** is centralized in `internal/api/mappers.go` as `toAPI<X>`.
  Flag a handler hand-building a `generated.X` inline.
- **Tracing** goes through `internal/trace` helpers (`trace.StartSpan`, not
  `span.StartChild`). File layout: one Ent type ↔ one `schema/<name>.go`, one
  `repository/<name>_repo.go` (interface + lowercase impl + `ec` helper), one
  `service/<name>.go`, handlers grouped `api/handlers_<area>.go`.
- **Package-scoped error prefixes** (`crypto:`, `payrexx:`, `payment:`,
  `qrsign:`, `enttenant:`).

---

## 3. Web conventions

- **RSC discipline:** `page.tsx` stays a server component (fetches, metadata);
  interactivity goes in a sibling `*-client.tsx` behind `<Suspense>`. Exception:
  `app/admin/*` pages are fully `"use client"`, gated once by `app/admin/layout.tsx`'s
  `AdminGuard`. Flag a `page.tsx` marked `"use client"` that also fetches/exports
  metadata (outside admin).
- **No Server Actions.** All mutations go through the `/api` proxy via `fetch` /
  `lib/api/*` (so they keep CSRF + host pinning). Flag a new `"use server"`.
- **Browser calls use the relative `/api` proxy**; server code uses the internal
  backend URL via `resolveApiBase()`. Flag a client component fetching an absolute
  backend URL or `process.env.NEXT_PUBLIC_API_BASE_URL`.
- **Per-domain API wrappers** live in `lib/api/<domain>.ts`, own their DTOs, and
  normalize raw backend shapes. There is **no generated client** — backend types
  are hand-mirrored in `web/types/*` / `lib/api/*` (e.g. `lib/auth/rbac.ts`
  mirrors `backend/internal/auth/rbac.go`). Flag inline `fetch("/v1/…")` in a page
  when a wrapper exists, and duplicated DTOs.
- **Pages with tenant-specific live data** declare `export const dynamic =
"force-dynamic"` (branding/menu are per-tenant + runtime-env dependent).
- **Per-tenant branding** is fetched server-side (`getBranding`, React-`cache`d,
  merged over defaults), applied as CSS vars in the root layout `<style>` (FOUC-free)
  and via `useBranding` context for text/logos. Flag hardcoded brand
  name/logo/color, or bypassing this for branding.
- **Context trio:** typed context + `Provider` + `useX()` hook that throws if used
  outside its provider; value memoized; provider wraps the smallest subtree
  (cart only in `(site)`). **Hooks** live in `hooks/use-<thing>.ts`, are
  `"use client"`, and SSR-guard `window`/`localStorage`/`document` access.
- **Two auth-fetch hooks:** `useAuthorizedFetch` (Better Auth user, Bearer +
  credentials) vs `useDeviceFetch(type)` (device JWT, clears token + redirects to
  `/{type}/setup` on 401/403). Use the right one — don't hand-roll Bearer.
- **Styling:** `cva` variants + `cn()`; `React.ComponentProps<"x"> &
VariantProps`; theme via CSS vars (never hardcoded hex). Images use `next/image`
  (no raw `<img>`); new remote image hosts come from env, not hardcoded.
- **Types** live in `web/types/*` and are re-exported type-only through
  `types/index.ts`; wire shapes get a `*DTO` suffix. Strict TS +
  `noUncheckedIndexedAccess` + ts-reset — narrow `unknown`, don't cast to `any`.
- **User-facing copy is German (de_CH)**; errors are inline local state, not
  toasts. Namespace logs as `console.error("[scope] …")`. (Flag new English
  user-facing strings and new toast usage as drift — see "Legacy" re: `sonner`.)
- **Sentry env** is host-derived on the client / `APP_ENV` on the server, never
  build-time baked; reuse the `backend-proxy-cold-start` fingerprint for proxy
  cold-start errors.

---

## 4. Generated code & migrations

- **Generated code is never committed** (`backend/internal/generated/**`,
  `backend/docs/swagger.*`) — it's gitignored and regenerated. Flag any diff
  adding tracked generated files or hand-editing a file with a "DO NOT EDIT" /
  oapi / Ent banner.
- **Schema/spec is the source of truth.** Change `internal/schema` (Ent) or
  `openapi/openapi.yaml`, then regenerate (`just generate`) — never edit the
  artifact. Flag an endpoint added in Go without an `openapi.yaml` change, or
  changes to the Ent feature-flag list (`intercept`/`privacy` are load-bearing
  for RLS).
- **Migrations are Atlas-managed, `YYYYMMDDHHMMSS_<slug>.sql`, append-only**, with
  a committed `atlas.sum`, and must replay on **PostgreSQL 17** (prod is PG17 —
  no PG18-only SQL). Flag: edits to an already-applied migration; a new migration
  without `atlas.sum` update (`just migrate-hash`); non-timestamp filenames.
- **A new tenant-scoped table** must, in the same migration, add `tenant_id`
  (+index +FK), re-scope any global unique constraint to `(tenant_id, …)`, `ENABLE
ROW LEVEL SECURITY`, and create an isolation policy `USING/WITH CHECK (tenant_id
= app_current_tenant_id() OR app_is_platform_admin())`. No `FORCE ROW LEVEL
SECURITY` (the owner must bypass). Cryptographic-secret unique keys
  (`token_hash`, `claim_token`) stay **globally** unique by design — don't
  per-tenant-scope those.
- **Seed files** set a transient `tenant_id` DEFAULT (`'tenant_seed1'`), insert
  with `ON CONFLICT … DO NOTHING` and fixed nanoid ids, then DROP the default.
  Flag a seed inserting into a scoped table without this dance.

---

## 5. Android

- **Single neutral app.** `applicationId`/namespace (`ch.leys.comanda`) and
  launcher name are compiled and brand-free; build types vary only `app_name`
  suffix + `applicationIdSuffix`. Flag `productFlavors`, per-tenant
  `applicationId`, or any brand string/asset baked into resources.
- **Branding is fetched at runtime** from the tenant (receipt logo via
  `BrandingAssets`, receipt fields from the web POS print payload) with neutral
  fallbacks only. Flag a hardcoded receipt address/logo/business name.
- **`WebPosActivity` is the launcher** (WebView shell loading `BuildConfig.POS_URL`);
  native↔web go through the `PosBridge` `@JavascriptInterface` and `comanda:*`
  CustomEvents with correlation ids; only the configured host loads in-app. Flag a
  new launcher, native screens duplicating web UI, or bridge results off the
  `comanda:*`/correlation contract. (`MainActivity`/`PosWebActivity` are legacy —
  see "Legacy".)
- **WebView is hardened by default;** cleartext/mixed-content/3rd-party-cookies are
  gated behind `BuildConfig.DEV_BUILD`, and cleartext config lives only in the
  `dev` source set. Flag enabling any of these unconditionally or in `main`/release.
- **OTA self-update verifies SHA-256 before install**; Bluetooth ops are
  permission-guarded (SDK-31) and fail-soft. Flag skipping the digest check or
  unguarded BT calls.
- New Android deps go in the version catalog (`libs.versions.toml`) as `libs.*`
  (a few JitPack/one-off coordinates are grandfathered).

---

## 6. Build, CI, secrets, git

- **No secrets in the repo.** `.env*` is gitignored except `.env.example`; Android
  secrets live in gitignored `secrets.properties`/`local.properties`/`*.jks`.
  Flag any tracked secret file, hardcoded key/token/password, or a real value in
  `.env.example`.
- **Release builds fail closed** when required secrets/URLs are absent (Android
  `throw GradleException`). Flag removing/weakening those guards or adding a
  hardcoded fallback.
- **Third-party GitHub Actions are pinned to a full commit SHA** with a `# vX`
  comment (first-party Anthropic/dependabot helpers may use tags). Flag a new
  third-party action pinned by mutable tag/branch in the CI/build workflows.
- **GitOps boundary:** this repo builds linux/amd64 images to
  `ghcr.io/ly-schneider/comanda/*` tagged `sha-<commit>` and dispatches to the
  `hetzner` repo — no in-repo deploy. Flag Azure references, `:latest`/branch image
  tags, or `kubectl/terraform apply` deploy steps added here.
- **Conventional Commits**, subject-only, terse; allowed types only; **no
  `Co-Authored-By`/AI/Generated-by trailers**. Branch naming `ley-<ticket#>-<slug>`
  (no user prefix, no separate worktree dirs).
- **`just` is the canonical task runner**; tools are version-pinned into
  `backend/bin/`. Flag a recipe using unpinned `go install …@latest`.

---

## 7. Comment policy (enforce strictly)

- Flag every code comment that merely restates what the code already says,
  narrates the steps, or labels an obvious block.
- A comment is justified **only** when it explains something that cannot be read
  from the code itself: a non-obvious _why_, a gotcha, an external constraint, or
  a subtle invariant. Prefer clear names over explanatory comments; when in doubt,
  remove it.
- Flag transitional/historical comments ("was X, now Y", "// NEW", "moved from…")
  and leftover phase/ticket references (e.g. `LEY-000`, `(P4)`, `TODO(P#)`) in
  _new or edited_ code — code should read as if the feature was always there.
  (Phase markers baked into already-applied migration filenames are grandfathered.)

## 8. Testing

- **Require tests for all new logic**, not just security-critical code.
- Security/tenancy-critical changes need the matching nets: new scoped table → an
  RLS isolation test; changes to data access → assertions on the RLS-bound _app_
  pool (not just the privileged pool); crypto/qrsign/redeem → adversarial unit
  tests (tamper, wrong key, wrong tenant, unknown version). Integration tests run
  the real Atlas migration dir (so RLS policies are exercised) — flag a test that
  builds schema via `client.Schema.Create`.
- Flag adding an allowlist entry (`allowlistNoRLS`, `globalPermissiveTables`,
  `csrfExempt`) purely to dodge a failing invariant test.
- Co-locate web tests as `*.test.ts(x)` next to source. Playwright/Storybook are
  scaffolded but dormant — don't _demand_ e2e/stories for a feature.

## Legacy (don't extend, don't nag)

These exist but are dead/scaffolding. Don't flag them as-is; **do** flag new code
that builds on or imitates them:

- Android `MainActivity` / `PosWebActivity` (legacy demo: hardcoded totals, a
  hardcoded printer MAC, narration comments, `println`, `https://app.example`).
  Only `WebPosActivity` is real.
- Backend `internal/response` `ProblemDetails` / `WriteSuccess`/`WriteProblem`
  (unused by handlers) and the vestigial `repository.Repositories` struct + `New()`.
- Web dual auth re-export shims (`lib/auth/index.ts`, `lib/auth/client.ts`), the
  unused `sonner` dependency, and the empty Playwright/Storybook setup.
