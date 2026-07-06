# CLAUDE.md — Comanda multi-tenant build

> This repo is the de-branded, multi-tenant evolution of `bless2n-food-system` (BlessThun).
> It is **platform-tenant #2** on the shared Leys Hetzner K3s platform (sibling to `iglesio`).
> Full design + decisions: Linear epic **LEY-308**; design doc attached there and at
> `../comanda-platform-design.md`.

## Code style

- **No unnecessary comments.** Code should speak for itself; comment only genuinely surprising logic.
- Match the conventions of the surrounding sub-project.

## Repository layout (de-branded from `bfs-*` in P0)

```
bfs-backend/   → backend/    Go + Ent + Chi (Clean Arch, Uber FX). RLS lives here.
bfs-web-app/   → web/        Next.js 16 / React 19. Host-based tenant resolution + Better Auth.
bfs-android-app/ → android/  Kotlin POS. Single neutral app; in-app branding dynamic.
bfs-cloud/     → DELETED in P0       (Azure Terraform — replaced by Hetzner gitops in the `hetzner` repo)
bfs-docs/      → docs/       Fumadocs.
```

## The build: 8 phases, one Linear ticket each (epic LEY-308)

| Phase  | Ticket  | Scope                                                                                                                       | Depends on  |
| ------ | ------- | --------------------------------------------------------------------------------------------------------------------------- | ----------- |
| **P0** | LEY-309 | Repo bootstrap & de-brand (rename `bfs-*`→ de-prefixed `backend`/`web`/`docs`/`android`, strip Azure, neutral placeholders) | —           |
| **P1** | LEY-310 | Tenancy data layer: `tenant` table, `tenant_id`+RLS everywhere, 2 DB roles, Ent tx-per-request + interceptor                | P0          |
| **P2** | LEY-311 | Tenant resolution (host-based) + Better Auth org plugin, per-tenant customers, dynamic trustedOrigins                       | P1          |
| **P3** | LEY-312 | BYO branding: per-tenant settings, R2 logo, replace hardcoded brand, neutral seed                                           | P1, P2      |
| **P4** | LEY-313 | BYO Payrexx: per-tenant creds (AES-256-GCM), per-tenant client, scoped webhooks, `PaymentProvider` iface                    | P1, P3      |
| **P5** | LEY-314 | Infra on Hetzner: gitops manifests, Scaleway DB, Doppler, ArgoCD, CI→GHCR SHA tags                                          | P0, P1      |
| **P6** | LEY-315 | BYO custom domains: HTTP-01 issuer, verification flow, API-driven dynamic Ingress controller                                | P2, P5      |
| **P7** | LEY-316 | Onboarding & platform admin (Leys-provisioned; billing/self-serve deferred)                                                 | P2,P3,P4,P6 |

Dependency graph (parallel windows): after **P0**, **P1** and P5-scaffolding can begin; after **P1**, **P2 ∥ P5**; P3→P4 chain off P2.

## Non-negotiable principles

1. **RLS is the isolation boundary.** Two DB roles: privileged owner (migrations, owns tables → bypasses RLS) + RLS-scoped app role (request traffic). Per request: single pgx tx → `set_config('app.tenant_id', $1, true)` → queries. Mirror the **iglesio** pattern (`../iglesio`, `../hetzner/infra/modules/db`).
2. **Never leak across tenants.** Every tenant table gets an RLS policy; a policy-coverage test + cross-tenant integration tests are part of P1's definition of done.
3. **Payment secrets never in plaintext.** AES-256-GCM, master key from Doppler; ciphertext in DB.
4. **Webhook routing is per-tenant** (`/payments/webhooks/payrexx/{tenantToken}`) — wrong-tenant verification = payment misattribution.
5. **No Azure.** Deploys are Hetzner K3s via ArgoCD gitops (the `hetzner` repo), images `ghcr.io/ly-schneider/comanda/*` pinned by commit SHA.
6. **Backend HTTP is OpenAPI-first.** New/changed endpoints go through the `oapi-codegen` wrapper — **never** hand-wired `apiHandlers.X` (or a closure) in `backend/internal/http/router.go` (only `/health` + `/ping` are exempt). Add a path + schema under `backend/openapi/` with `operationId` == the Go handler method name, run `just generate-api`, then wire `wrapper.X` in the right middleware group. `server.gen.go` / `openapi/bundled.yaml` are gitignored (regenerated, not committed).

## Subagents (`.claude/agents/`)

- `de-brand-sweeper` — P0 mechanical rename/strip sweep.
- `go-backend-dev` — P1/P4 Go+Ent: RLS, tx-per-request, payments.
- `nextjs-web-dev` — P2/P3 Next.js: host resolution, Better Auth, branding.
- `android-dev` — P3 Kotlin: neutral app + dynamic in-app branding.
- `gitops-infra-dev` — P5/P6 Hetzner gitops, ArgoCD, Traefik/cert-manager, the domain controller.
- `rls-security-auditor` — read-only, plan-mode audits after P1 & P4.

## Orchestration rules (for the lead session)

- **One task → one branch, checked out in the normal repo dir** (`/Users/levyn/Developer/comanda`), based off `main`. Do **not** create separate worktree directories — work the branch in place so tests run against the installed toolchain (pnpm/`node_modules`, DB, lint) and the human can test it directly without switching dirs.
- Run **one phase at a time**, gated on human review. Sequential by default (the in-place branch holds one task at a time); this trades the old P2 ∥ P5 parallelism for autonomous verifiability.
- Spawn risky phases (**P1, P4, P6**) in **plan mode** — agent proposes a plan, STOPS for approval, then implements.
- Pass the **Linear ticket ID** into every subagent prompt: it sets `In Progress` on start, posts a progress comment, flips to `In Review` with a summary when done. The lead flips the epic LEY-308.
- After **P1** and **P4**, run `rls-security-auditor` before advancing.

## Git conventions

- **Commits are GPG-signed.** This repo inherits the global `commit.gpgsign = true`; the key is reachable in the dev/agent environment. **Do not** set `commit.gpgsign false` locally (a past bootstrap did this and left unsigned, unverified commits). If a non-interactive run genuinely can't reach the key, fix the agent's key access rather than disabling signing.
- **Never push to origin.** Work commits on the **task branch** (checked out in the main repo dir); the human does merge-to-`main` / push after review. Never commit feature work straight to `main` or rewrite shared history.
- **Branch naming:** `ley-<ticket#>-<slug>` (e.g. `ley-324-backend-ed25519-qr-signing`). No `levynschneider/` (or any user) prefix. **No separate worktree dirs** — older phases used `../comanda-ley-*` worktrees; new tasks are branches checked out in place.
- **Commit messages:** conventional-commit subject line, kept terse. No `Co-Authored-By` trailer.

## Local environment

- Docker Desktop gets restarted frequently. If the Docker daemon is unreachable, stop and say so instead of retrying — Levyn will tell you when it's back up.

## Dev / build commands (carried over; paths update after P0)

```bash
# Backend (Go)
cd backend && just up && just migrate && just dev   # API :8080
just test            # unit       |   just test-integration  # needs PG
just lint --fix && just fmt

# Web (Next.js, pnpm)
cd web && pnpm install && pnpm dev                   # :3000
pnpm typecheck && pnpm lint && pnpm test && pnpm e2e:headless
```
