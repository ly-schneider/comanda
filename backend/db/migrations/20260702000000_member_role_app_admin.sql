-- Add the app_admin tier between admin and staff: full app management (users,
-- invites, food-system settings) but hard-blocked from branding, domains, and
-- payments. Extend both the member.role and admin_invite.member_role check sets.

ALTER TABLE "admin_invite" DROP CONSTRAINT "admin_invite_member_role_check";
ALTER TABLE "admin_invite" ADD CONSTRAINT "admin_invite_member_role_check"
    CHECK (member_role IN ('admin', 'app_admin', 'staff', 'catalog'));

ALTER TABLE "member" DROP CONSTRAINT "member_role_check";
ALTER TABLE "member" ADD CONSTRAINT "member_role_check"
    CHECK (role IN ('owner', 'admin', 'app_admin', 'staff', 'catalog'));
