-- The settings table is created in the baseline; its grant to the RLS app role
-- was role-guarded and got skipped on any environment where comanda_app was
-- created after the baseline ran (same failure mode as 20260623 for the Better
-- Auth tables). The app role does all request traffic against settings — reads
-- branding, encrypts/decrypts the payment + email secrets, and inserts the
-- per-tenant singleton during platform provisioning (SeedSettings). Without the
-- grant that insert fails with "permission denied for table settings" (42501),
-- surfacing as an opaque 500, and tenant-host settings reads fail too. Grant it
-- explicitly. Role-guarded so the roleless CI replay (atlas migrate apply on a
-- bare postgres) is a no-op.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON settings TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON settings TO comanda_privileged;
  END IF;
END$$;
