-- Owner-invites are backend-managed: a Leys platform admin invites a vendor
-- owner for a pre-created tenant, the owner registers via OTP, and accepts the
-- invite to become the org owner. They reuse the Better Auth invitation table but
-- carry a hashed token and an acceptance timestamp the plugin never writes.
ALTER TABLE "invitation" ADD COLUMN "token_hash" TEXT;
CREATE UNIQUE INDEX "invitation_token_hash_key" ON "invitation" ("token_hash") WHERE "token_hash" IS NOT NULL;
ALTER TABLE "invitation" ADD COLUMN "accepted_at" TIMESTAMPTZ;

-- One membership per (org, user): anchors idempotent member inserts on accept
-- and the backfill below.
CREATE UNIQUE INDEX "member_org_user_key" ON "member" ("organization_id", "user_id");

-- Staff invites grant an ops member by default; an invite may instead grant a
-- full tenant admin. Existing accepted invites default to 'admin' so backfilled
-- members keep their current full access.
ALTER TABLE "admin_invite" ADD COLUMN "member_role" TEXT NOT NULL DEFAULT 'admin'
    CHECK ("member_role" IN ('admin', 'ops'));

-- Local nanoid generator for the backfill (12 chars, alphabet excludes
-- 0 O o I i J j L l). Lives in pg_temp so it vanishes with the session.
CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZ_abcdefghkmnpqrstuvwxyz-',
                           (floor(random()*55))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

-- Link any tenant that predates the organization link to a fresh organization.
-- The subdomain /admin gate filters member rows on app_current_org_id(), which
-- is NULL for an unlinked tenant — so without this every admin of such a tenant
-- would be locked out. The slug carries the tenant id to dodge organization.slug
-- being UNIQUE.
DO $$
DECLARE
  r RECORD;
  v_org_id varchar(12);
BEGIN
  FOR r IN SELECT id, slug, name FROM tenant WHERE organization_id IS NULL LOOP
    v_org_id := pg_temp.nanoid();
    INSERT INTO "organization" (id, name, slug, created_at)
      VALUES (v_org_id, r.name, r.slug || '-' || r.id, now());
    UPDATE tenant SET organization_id = v_org_id WHERE id = r.id;
  END LOOP;
END $$;

-- Backfill members from already-accepted staff invites so existing admins keep
-- access once the gate becomes membership-driven. Their full access is preserved
-- via the member_role default of 'admin'.
INSERT INTO "member" (id, organization_id, user_id, role, created_at)
SELECT pg_temp.nanoid(), t.organization_id, u.id, COALESCE(ai.member_role, 'admin'), now()
FROM admin_invite ai
JOIN tenant t ON t.id = ai.tenant_id
JOIN "user" u ON lower(u.email) = lower(ai.invitee_email)
WHERE ai.status = 'accepted' AND t.organization_id IS NOT NULL
ON CONFLICT (organization_id, user_id) DO NOTHING;

-- The bootstrap tenant's owner has no admin_invite row to derive from (there is
-- no SQL-seeded admin user), so their membership cannot be backfilled
-- automatically. Confirm the owner email at deploy time and run the insert below.
--
-- DEPLOY: set the bootstrap owner email, then uncomment.
-- INSERT INTO "member" (id, organization_id, user_id, role, created_at)
-- SELECT (
--          SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZ_abcdefghkmnpqrstuvwxyz-',
--                                   (floor(random()*55))::int + 1, 1), '')
--          FROM generate_series(1, 12)
--        ),
--        t.organization_id, u.id, 'owner', now()
-- FROM tenant t
-- JOIN "user" u ON lower(u.email) = lower('OWNER_EMAIL_HERE')
-- WHERE t.id = 'tenant_seed1' AND t.organization_id IS NOT NULL
-- ON CONFLICT (organization_id, user_id) DO NOTHING;
