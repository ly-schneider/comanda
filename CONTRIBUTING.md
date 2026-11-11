# Contributing

Thanks for helping. A few things keep the project consistent.

## Before you start

For anything bigger than a small fix, open an issue first so we can agree on the approach. Each sub-project's README (`backend/`, `web/`, `android/`, `docs/`) describes its architecture and conventions.

## Workflow

1. Follow the Quick Start in the README to get the backend and web app running.
2. Commits use Conventional Commits: `type(scope): summary`. Versions are derived from them, so `feat` bumps the minor version, `fix` the patch, and `feat!` the major.
3. Backend HTTP is OpenAPI-first: add or change endpoints under `backend/openapi/` (with `operationId` matching the Go handler method), run `just generate-api`, and wire the generated wrapper. Don't hand-wire routes.
4. Every tenant-owned table needs `tenant_id` and a Row-Level Security policy. The policy-coverage and cross-tenant tests must pass.
5. Before opening a PR:
   - `cd backend && just lint && just test`
   - `cd web && pnpm typecheck && pnpm lint && pnpm test`

## Licensing of contributions

Comanda is licensed under the [Business Source License 1.1](LICENSE), and the licensor also sells commercial licenses. To keep that possible, by submitting a contribution you agree that:

- you wrote it or otherwise have the right to submit it, and
- you grant Leys Services a perpetual, worldwide, non-exclusive, royalty-free, irrevocable license to use, modify, sublicense and distribute your contribution under the project's license and under any other license, including commercial ones.

You keep the copyright to your contribution.
