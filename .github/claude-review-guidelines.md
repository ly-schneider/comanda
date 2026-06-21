# Claude code-review guidelines

Canonical, human-editable version of the review context. The same prose is
mirrored into the `--append-system-prompt` value in
`.github/workflows/claude-code-review.yml` so it applies for the whole review
regardless of how the `/code-review` slash command is processed. **Edit here
first, then mirror into the workflow.**

## Stack

- **Backend** — Go + Ent + Chi, Clean Architecture, Uber FX DI, on Postgres.
  Row-Level Security (RLS) is the tenant-isolation boundary.
- **Web** — Next.js 16 / React 19, Better Auth, host-based tenant resolution.
  `proxy.ts` is the active middleware (not `middleware.ts`).
- **Android** — a single neutral Kotlin POS app; all in-app branding is dynamic.
- **Infra** — Hetzner K3s via ArgoCD gitops; images pinned by commit SHA. No Azure.

## Security invariants (treat any violation as a blocking finding)

1. **RLS is the isolation boundary.** Two DB roles: a privileged owner that
   bypasses RLS for migrations, and an RLS-scoped app role for request traffic.
   Every request runs a single pgx transaction that sets `app.tenant_id` via
   `set_config` before any query.
2. **No cross-tenant access.** Every tenant-scoped table has an RLS policy; no
   query path may read or write across tenants.
3. **Payment secrets never in plaintext.** Payrexx creds are AES-256-GCM
   encrypted with the master key from Doppler; only ciphertext is stored.
4. **Webhooks route per tenant** via a tenant token in the path. Verifying
   against the wrong tenant misattributes payments.

## Method

- Explain the **why** behind every finding, not just the what.
- Tie each finding to one of the invariants above or to a concrete correctness,
  security, or data-loss risk.
- Match each sub-project's existing conventions instead of imposing new ones.
- Flag missing RLS tests or cross-tenant leak risk on any change to data access.

## Comment policy (enforce strictly)

- Flag every code comment that merely restates what the code already says,
  narrates the steps, or labels an obvious block.
- A comment is justified **only** when it explains something that cannot be read
  from the code itself: a non-obvious _why_, a gotcha, an external constraint, or
  a subtle invariant.
- Prefer clear names over explanatory comments; when in doubt, remove it.
- Flag transitional/historical comments and leftover ticket references (e.g.
  `LEY-000`) — code should read as if the feature was always there.
