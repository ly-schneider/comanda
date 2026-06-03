-- better_auth_org_plugin.sql
-- P2 (LEY-311): Better Auth organization plugin tables (org = tenant identity
-- carrier). Auth-plane tables: same global/permissive RLS as user/session/
-- account/verification. Tenant-data isolation lives in the scoped tables.

CREATE TABLE IF NOT EXISTS "organization" (
    "id"         TEXT NOT NULL PRIMARY KEY,
    "name"       TEXT NOT NULL,
    "slug"       TEXT NOT NULL UNIQUE,
    "logo"       TEXT,
    "metadata"   TEXT,
    "created_at" TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS "member" (
    "id"              TEXT NOT NULL PRIMARY KEY,
    "organization_id" TEXT NOT NULL REFERENCES "organization" ("id") ON DELETE CASCADE,
    "user_id"         TEXT NOT NULL REFERENCES "user" ("id") ON DELETE CASCADE,
    "role"            TEXT NOT NULL,
    "created_at"      TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS "member_organization_id_idx" ON "member" ("organization_id");
CREATE INDEX IF NOT EXISTS "member_user_id_idx" ON "member" ("user_id");

CREATE TABLE IF NOT EXISTS "invitation" (
    "id"              TEXT NOT NULL PRIMARY KEY,
    "organization_id" TEXT NOT NULL REFERENCES "organization" ("id") ON DELETE CASCADE,
    "email"           TEXT NOT NULL,
    "role"            TEXT,
    "status"          TEXT NOT NULL DEFAULT 'pending',
    "inviter_id"      TEXT NOT NULL REFERENCES "user" ("id") ON DELETE CASCADE,
    "expires_at"      TIMESTAMPTZ NOT NULL,
    "created_at"      TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS "invitation_organization_id_idx" ON "invitation" ("organization_id");
CREATE INDEX IF NOT EXISTS "invitation_email_idx" ON "invitation" ("email");

ALTER TABLE "session" ADD COLUMN IF NOT EXISTS "active_organization_id" TEXT;

ALTER TABLE "organization" ENABLE ROW LEVEL SECURITY;
CREATE POLICY "organization_all" ON "organization" FOR ALL USING (true) WITH CHECK (true);
ALTER TABLE "member" ENABLE ROW LEVEL SECURITY;
CREATE POLICY "member_all" ON "member" FOR ALL USING (true) WITH CHECK (true);
ALTER TABLE "invitation" ENABLE ROW LEVEL SECURITY;
CREATE POLICY "invitation_all" ON "invitation" FOR ALL USING (true) WITH CHECK (true);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON "organization" TO comanda_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON "member" TO comanda_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON "invitation" TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON "organization" TO comanda_privileged;
    GRANT ALL ON "member" TO comanda_privileged;
    GRANT ALL ON "invitation" TO comanda_privileged;
  END IF;
END$$;
