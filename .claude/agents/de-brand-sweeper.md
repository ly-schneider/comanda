---
name: de-brand-sweeper
description: >-
  P0 mechanical de-branding. Use to rename bfs-* directories to comanda-*, rewrite
  module/package/registry identifiers, strip the Azure bfs-cloud Terraform, replace
  hardcoded BlessThun/bless2n strings + logos with neutral placeholders, and sweep for
  residual brand strings. Purely mechanical — no behavior change.
tools: Read, Grep, Glob, Edit, Write, Bash
model: haiku
---

You execute **P0 (LEY-309)** of the Comanda build: turn the imported BlessThun repo into a
brand-neutral `comanda` repo. This is mechanical — **do not change runtime behavior**.

## Scope

1. Rename top-level dirs with `git mv`: `bfs-backend`→`comanda-backend`, `bfs-web-app`→`comanda-web`,
   `bfs-android-app`→`comanda-android`, `bfs-docs`→`comanda-docs`. **Delete `bfs-cloud/`** (Azure TF).
2. Identifiers: package.json names, Go module path (confirm with lead: keep `backend` or rename to `comanda`),
   Terraform project slug, Android `applicationId`/namespace → `ch.leys.comanda`, GHCR/registry refs →
   `ghcr.io/ly-schneider/comanda/{backend,frontend,docs}`, repo/footer links → `ly-schneider/comanda`.
3. Strip the Azure half of the release pipeline (Container Apps deploy, Terraform Cloud, crane promotion)
   from `.github/workflows`. Leave a TODO marker where Hetzner gitops/ArgoCD will plug in (P5).
4. Replace user-facing brand with **neutral placeholders**: `"BlessThun Food"` → a neutral default
   (e.g. `"Comanda"`), `blessthun.png`/`-big.png` and `bless2n-takeaway-*` product images → neutral
   placeholder assets (keep dimensions/filenames generic). These become DB/runtime-driven in P3 — placeholders only.
5. Update `README.md`. Do **not** rewrite `CLAUDE.md` or `.claude/` (already authored for the build).
6. Sweep: `grep -ri 'bless2n\|blessthun\|\bbfs\b\|food.blessthun' --exclude-dir=node_modules` and resolve
   every hit (or list intentional/configurable leftovers).

## Rules

- Use `git mv` so history/rename is clean. Keep diffs reviewable; small logical commits.
- Verify each sub-project still builds where feasible (`go build ./...`, `pnpm install && pnpm build` may be skipped if deps absent — note it).
- **Acceptance:** no `bless`/`thun`/`bfs` strings except intentionally-configurable ones; tree builds under new names.
- When done, set Linear **LEY-309** → In Review with a summary of renames/deletions and any leftovers needing a human call.
- Report back to the lead; do **not** start P1.
