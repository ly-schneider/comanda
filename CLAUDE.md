# Comanda

Comanda is a multi-tenant till and phone ordering system for food stands at events. Each organiser is a tenant with their own shop, domain, branding and payment accounts. The README explains the product; this file covers how to work in the repo.

## Layout

- `backend/`: Go API (Chi, Ent, Uber FX), PostgreSQL with row-level security, Atlas migrations.
- `web/`: Next.js 16 app with the storefront, admin, till screens, pickup station, organiser hub and the marketing site on the platform host.
- `android/`: Kotlin till app, a WebView shell around the web till with SumUp and Bluetooth receipt printing.
- `docs/`: Fumadocs documentation site.
- `.github/workflows/`: `pr-checks.yml`, `main.yml` and the manual `android-release.yml`.

Rules for each area live in `.claude/rules/` and load when you work on matching files.

## Commands

```bash
cd backend && just up                       # Postgres, MinIO, Mailpit in Docker
cd backend && just bootstrap-local <email>  # migrations, demo catalog, admin user
cd backend && just dev                      # API on :8080
cd backend && just test                     # unit tests
cd backend && just test-integration         # needs Postgres
cd backend && just lint && just fmt
cd backend && just generate                 # Ent and OpenAPI code

cd web && pnpm install && pnpm dev          # app on :3000
cd web && pnpm typecheck && pnpm lint && pnpm test
cd web && pnpm e2e:headless
```

Docker Desktop is restarted often. If the Docker daemon doesn't answer, stop and say so instead of retrying.

## Rules that apply everywhere

- Tenant isolation is enforced by PostgreSQL row-level security. Never add a path that reads or writes tenant data outside it.
- Payment credentials are stored encrypted. Never log or store them in plaintext.
- Write code as if it had always been this way. No comments about history, no ticket numbers in code or file names, and comments only for what the code can't say itself.
- User-facing copy is Swiss Standard German in the du form: `ss` instead of `ß`, numbers like `4'051`.
- Never touch the staging or production database unless asked for that exact query. Propose the SQL instead.

## Git

- Work on a branch off `main`, checked out in this directory. Don't create separate worktree folders.
- Commits are signed. Never disable `commit.gpgsign`; fix key access instead.
- Commit messages are Conventional Commits with a short subject line and no trailers.
- Never push unless asked in the current message. Pull requests are squash-merged, so the PR title becomes the commit on `main`.

## CI and deploys

`pr-checks.yml` runs only the checks for the parts that changed and ends in one "All checks" job. `main.yml` runs the same checks on every push to `main`, builds images for the parts that changed, deploys them to staging through the `ly-schneider/hetzner` gitops repo and then waits for approval on the `production` environment. Android builds are manual through `android-release.yml`.
