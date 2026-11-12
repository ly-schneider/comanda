---
paths:
  - "backend/**"
---

# Backend

- HTTP endpoints are OpenAPI-first. Add the path and schema under `backend/openapi/` with `operationId` equal to the Go handler method name, run `just generate-api`, then wire `wrapper.X` in the right middleware group in `internal/http/router.go`. Only `/health` and `/ping` are wired by hand. `server.gen.go` and `openapi/bundled.yaml` are generated and gitignored.
- Services reach the database through `repository.ClientFromContext(ctx, s.client)`, never `s.client` directly. Without it the request transaction and its `app.tenant_id` setting are missing, and row-level security returns no rows.
- Each request runs in one database transaction on one connection. Never run queries on it concurrently, for example with `errgroup`.
- Every new tenant table gets a `tenant_id` column and a row-level security policy. `test/integration/rls_coverage_test.go` fails when a table has none.
- Migrations are Atlas files in `db/migrations/`. After editing one, run `just migrate-hash`.
- Old pods keep running during a rollout, so a destructive change such as dropping a column ships in two releases: first stop using it, then drop it.
- Ent code is generated with `just generate-ent`, which runs `go tool ent`.
- IDs are 12-character nanoids generated in the app (`internal/id`).
