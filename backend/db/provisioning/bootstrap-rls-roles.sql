-- bootstrap-rls-roles.sql
-- RLS role bootstrap for Comanda. Run by a superuser / instance owner BEFORE the
-- first migration; local dev runs it from the Compose init mount on a fresh
-- volume. Idempotent — re-run to repair ownership or grant drift.
--
-- Two roles (mirrors the iglesio pattern):
--   * comanda_privileged — owns every public table, so it bypasses RLS via
--     OWNERSHIP (no FORCE ROW LEVEL SECURITY is set). Runs migrations, cron,
--     auth-bootstrap, and tenant resolution. On platforms that allow it (local
--     dev, self-managed PG) it is additionally granted BYPASSRLS; on managed
--     PG that forbids BYPASSRLS, ownership alone is sufficient.
--   * comanda_app — non-owner, NON-BYPASSRLS role that serves /v1 request
--     traffic. Every policy filters on app.tenant_id, set per-request in a tx.
--
-- DSNs:
--   DATABASE_URL      -> comanda_privileged
--   DATABASE_URL_APP  -> comanda_app

-- =====================================================================
-- 1. Roles
-- =====================================================================

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    CREATE ROLE comanda_app NOINHERIT LOGIN PASSWORD 'REPLACE_ME_APP_PASSWORD';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    CREATE ROLE comanda_privileged NOINHERIT LOGIN PASSWORD 'REPLACE_ME_PRIVILEGED_PASSWORD';
  END IF;
END$$;

-- Grant BYPASSRLS to the privileged role where the platform permits it.
-- Ownership already bypasses RLS, so this is belt-and-suspenders; it is wrapped
-- so the script still succeeds on managed PG that forbids ALTER ... BYPASSRLS.
DO $$
BEGIN
  BEGIN
    ALTER ROLE comanda_privileged BYPASSRLS;
  EXCEPTION WHEN insufficient_privilege OR feature_not_supported THEN
    RAISE NOTICE 'Could not grant BYPASSRLS to comanda_privileged; relying on table ownership to bypass RLS.';
  END;
END$$;

-- =====================================================================
-- 2. Reassign existing object ownership to the privileged role so policy
--    creation, DEFAULT PRIVILEGES, and ownership-based RLS bypass all behave
--    predictably. The current owner must grant membership to CURRENT_USER on
--    managed PG; locally CURRENT_USER is a superuser.
-- =====================================================================

DO $$
BEGIN
  BEGIN
    EXECUTE format('GRANT comanda_privileged TO %I', CURRENT_USER);
  EXCEPTION WHEN OTHERS THEN
    NULL;
  END;
END$$;

DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT tablename FROM pg_tables WHERE schemaname = 'public'
  LOOP
    EXECUTE format('ALTER TABLE public.%I OWNER TO comanda_privileged', r.tablename);
  END LOOP;

  FOR r IN SELECT sequence_name FROM information_schema.sequences WHERE sequence_schema = 'public'
  LOOP
    EXECUTE format('ALTER SEQUENCE public.%I OWNER TO comanda_privileged', r.sequence_name);
  END LOOP;

  FOR r IN
    SELECT p.proname, pg_get_function_identity_arguments(p.oid) AS args
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'public'
  LOOP
    EXECUTE format('ALTER FUNCTION public.%I(%s) OWNER TO comanda_privileged', r.proname, r.args);
  END LOOP;
END$$;

-- =====================================================================
-- 3. Safety-net timeouts for the app role (every /v1 request runs in a tx).
-- =====================================================================

ALTER ROLE comanda_app SET statement_timeout = '30s';
ALTER ROLE comanda_app SET idle_in_transaction_session_timeout = '60s';

-- =====================================================================
-- 4. Schema usage + DML grants. RLS does the row filtering.
-- =====================================================================

GRANT USAGE ON SCHEMA public TO comanda_app, comanda_privileged;
GRANT CREATE, USAGE ON SCHEMA public TO comanda_privileged;

GRANT ALL ON ALL TABLES IN SCHEMA public TO comanda_privileged;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO comanda_privileged;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO comanda_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO comanda_app;

-- Future objects created by migrations (run as comanda_privileged) auto-grant
-- DML to the app role.
ALTER DEFAULT PRIVILEGES FOR ROLE comanda_privileged IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO comanda_app;
ALTER DEFAULT PRIVILEGES FOR ROLE comanda_privileged IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO comanda_app;

-- =====================================================================
-- 5. Atlas migration ledger. Atlas keeps it in its own schema, created by
--    whichever role first runs `atlas migrate`. If that is not
--    comanda_privileged, later runs fail with 42P01 on a table that does exist:
--    inspection reads pg_catalog (sees the table) while column privileges hide
--    its columns, so Atlas plans an ALTER against an invisible relation.
-- =====================================================================

DO $$
BEGIN
  EXECUTE format('GRANT CREATE ON DATABASE %I TO comanda_privileged', current_database());
EXCEPTION WHEN insufficient_privilege THEN
  RAISE NOTICE 'Could not grant CREATE on the database; comanda_privileged must own the atlas_schema_revisions schema for migrations to run.';
END$$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'atlas_schema_revisions') THEN
    ALTER SCHEMA atlas_schema_revisions OWNER TO comanda_privileged;

    IF EXISTS (
      SELECT 1 FROM pg_tables
      WHERE schemaname = 'atlas_schema_revisions' AND tablename = 'atlas_schema_revisions'
    ) THEN
      ALTER TABLE atlas_schema_revisions.atlas_schema_revisions OWNER TO comanda_privileged;
    END IF;
  END IF;
END$$;

-- =====================================================================
-- VERIFY (run by hand)
--   SELECT rolname, rolbypassrls, rolcanlogin FROM pg_roles
--     WHERE rolname IN ('comanda_app','comanda_privileged');
--   SELECT count(*) FROM pg_tables
--     WHERE schemaname='public' AND tableowner != 'comanda_privileged'; -- 0
-- =====================================================================
