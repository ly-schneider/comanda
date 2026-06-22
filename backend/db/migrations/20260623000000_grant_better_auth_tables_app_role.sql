-- The Better Auth core tables (created in the 20250101 baseline) are the global
-- auth plane: no tenant RLS, resolved before app.tenant_id is set. They were
-- never explicitly granted to the RLS app role and relied on the one-time
-- bootstrap GRANT, which misses environments where a table was (re)created after
-- bootstrap ran — leaving comanda_app unable to read `verification`, so the OTP
-- lookup fails with "permission denied for table verification". Grant them
-- explicitly. Role-guarded so the roleless CI replay (atlas migrate apply on a
-- bare postgres) is a no-op.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON "user", session, account, verification TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON "user", session, account, verification TO comanda_privileged;
  END IF;
END$$;
