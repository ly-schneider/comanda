-- Comprehensive catch-up grant for the RLS app role. comanda_app must hold DML
-- on every public table, but the blanket grant + ALTER DEFAULT PRIVILEGES that
-- normally provide it live in bootstrap-rls-roles.sql, which runs before the
-- schema exists. Where comanda_app was created after the baseline tables — or
-- where the default-privileges path never fired — the baseline Ent tables
-- (product, "order", menu, category, jeton, station, ...) stay ungranted and
-- every request that touches them fails with "permission denied for table X"
-- (42501), surfacing as an opaque 500. The 2026062{3,8,9} migrations patched
-- individual tables one at a time; this grants the whole schema in one shot and
-- re-asserts the default privileges so tables added by future migrations are
-- covered automatically. Runs as comanda_privileged (the migrate DSN), which
-- owns the tables, so the grants take effect. Role-guarded so the roleless CI
-- replay (atlas migrate apply on a bare postgres) is a no-op. Idempotent.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO comanda_app;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO comanda_app;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
      ALTER DEFAULT PRIVILEGES FOR ROLE comanda_privileged IN SCHEMA public
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO comanda_app;
      ALTER DEFAULT PRIVILEGES FOR ROLE comanda_privileged IN SCHEMA public
        GRANT USAGE, SELECT ON SEQUENCES TO comanda_app;
    END IF;
  END IF;
END$$;
