# Database Provisioning

Comanda uses two database roles, and RLS is the tenant isolation boundary:

| Role                 | DSN                | Purpose                                                                              |
| -------------------- | ------------------ | ------------------------------------------------------------------------------------ |
| `comanda_privileged` | `DATABASE_URL`     | Owns every table in `public`, so it bypasses RLS by ownership. Migrations, cron, auth bootstrap, tenant resolution. |
| `comanda_app`        | `DATABASE_URL_APP` | Non-owner, non-`BYPASSRLS`. Serves `/v1` request traffic; every policy filters on `app.tenant_id`, set per request inside the tx. |

## Local Development

Both roles are created on a fresh Postgres volume by the Compose init mounts —
`bootstrap-rls-roles.sql` first, then `docker-dev-passwords.sql` to swap the
`REPLACE_ME` placeholders for the dev passwords in `.env.local`.

```bash
just up          # roles bootstrapped on a fresh volume
just migrate     # apply migrations (as comanda_privileged)
just seed        # seed dev data
just dev         # start backend
```

To re-provision from scratch: `just down --volumes && just up`.

## New Remote Environment

Run the bootstrap as the instance owner / superuser **before** the first
migration, so migrations create their tables under `comanda_privileged` and the
default privileges are in place for `comanda_app`:

```bash
DATABASE_URL="postgres://owner:...@host/db" just bootstrap-rls
```

Then set real passwords and store them in Doppler:

```sql
ALTER ROLE comanda_privileged WITH PASSWORD '...';
ALTER ROLE comanda_app        WITH PASSWORD '...';
```

## Existing Environment

`bootstrap-rls-roles.sql` is idempotent and safe to re-run at any time. Re-run it
after a migration that created objects under the wrong owner, or when
`comanda_app` is missing DML grants — the symptom is a `42501` that the error
translator reports as a generic 500.

Verify:

```sql
SELECT rolname, rolbypassrls, rolcanlogin FROM pg_roles
  WHERE rolname IN ('comanda_app', 'comanda_privileged');

SELECT count(*) FROM pg_tables
  WHERE schemaname = 'public' AND tableowner != 'comanda_privileged'; -- 0
```

Atlas keeps its migration ledger in the `atlas_schema_revisions` schema, created
by whichever role first runs `atlas migrate`. If that is not
`comanda_privileged`, later runs fail with `relation "atlas_schema_revisions"
does not exist` — the table is there but invisible without `USAGE`. Fix by
transferring ownership:

```sql
ALTER SCHEMA atlas_schema_revisions OWNER TO comanda_privileged;
ALTER TABLE  atlas_schema_revisions.atlas_schema_revisions OWNER TO comanda_privileged;
```

## Files

| File                        | Purpose                                                        |
| --------------------------- | -------------------------------------------------------------- |
| `bootstrap-rls-roles.sql`   | Creates both roles, reassigns ownership, sets grants + default privileges and app-role timeouts |
| `docker-dev-passwords.sql`  | Local-dev passwords, applied after the bootstrap on `just up`   |
