-- Tenant role model: four nested tiers driven by member.role
-- (admin > staff > catalog) plus the owner singleton. "ops" is renamed to
-- "staff"; "catalog" is the new lowest tier. The inert global user.role (and the
-- unused Better Auth admin-plugin columns) are dropped — tenant authorization is
-- the member.role axis, platform authorization is is_platform_admin.

-- Rename the existing ops tier to staff across members and invites.
UPDATE "member" SET role = 'staff' WHERE role = 'ops';
UPDATE "admin_invite" SET member_role = 'staff' WHERE member_role = 'ops';

-- admin_invite.member_role: swap the old CHECK (member_role IN ('admin','ops'))
-- for the new tier set and default to staff. The original constraint was added
-- inline in 20260625000000, so Postgres named it admin_invite_member_role_check.
ALTER TABLE "admin_invite" DROP CONSTRAINT "admin_invite_member_role_check";
ALTER TABLE "admin_invite" ADD CONSTRAINT "admin_invite_member_role_check"
    CHECK (member_role IN ('admin', 'staff', 'catalog'));
ALTER TABLE "admin_invite" ALTER COLUMN member_role SET DEFAULT 'staff';

-- Codify the member tier set (owner is the admin-rank singleton).
ALTER TABLE "member" ADD CONSTRAINT "member_role_check"
    CHECK (role IN ('owner', 'admin', 'staff', 'catalog'));

-- Drop the inert global user.role (baseline: column, index, enum type).
DROP INDEX IF EXISTS idx_user_role;
ALTER TABLE "user" DROP COLUMN role;
DROP TYPE user_role;

-- Drop the unused Better Auth admin-plugin columns (zero callers).
ALTER TABLE "user" DROP COLUMN banned, DROP COLUMN ban_reason, DROP COLUMN ban_expires;
ALTER TABLE "session" DROP COLUMN impersonated_by;
