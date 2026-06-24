-- The organization-plugin tables (organization, member, invitation) are Better
-- Auth core tables that were never explicitly granted to the RLS app role —
-- 20260623 only covered user/session/account/verification. Platform tenant
-- provisioning inserts an organization, and the owner-invite accept flow inserts
-- a member + updates session, all on the request tx as comanda_app. Without these
-- grants those writes fail with "permission denied for table organization"
-- (42501), surfacing as an opaque 500. Role-guarded so the roleless CI replay
-- (atlas migrate apply on a bare postgres) is a no-op.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON "organization", "member", "invitation" TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON "organization", "member", "invitation" TO comanda_privileged;
  END IF;
END$$;
