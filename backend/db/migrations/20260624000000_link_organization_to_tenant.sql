-- link_organization_to_tenant.sql
-- Links each tenant 1:1 to its Better Auth organization and hardens the
-- org/member/invitation RLS now that the link exists.
--
-- Until now the backend resolved a tenant only from the request Host. The vendor
-- account hub lives on the platform host (no tenant Host), so it must resolve to
-- the logged-in vendor's tenant via session.active_organization_id. That needs a
-- mapping organization.id -> tenant.id, which this migration adds.
--
-- The org/member/invitation tables were RLS USING(true) (global), parked for the
-- onboarding work. With the link in place they become org-keyed: the comanda_app
-- request role sees only the org of the currently bound tenant (or everything as
-- a platform admin). Better Auth keeps writing them through the privileged pool,
-- which bypasses RLS by table ownership, so login/org flows are unaffected.

-- =====================================================================
-- LINK: tenant.organization_id -> organization.id (1:1).
-- TEXT to match organization.id (Better-Auth-shaped TEXT). Nullable: a tenant
-- provisioned before this migration (or mid-create) may be unlinked; a unique
-- index permits many NULLs but at most one tenant per organization.
-- =====================================================================

ALTER TABLE tenant ADD COLUMN organization_id TEXT;
CREATE UNIQUE INDEX tenant_organization_id_key ON tenant (organization_id);
ALTER TABLE tenant
  ADD CONSTRAINT tenant_organization_id_fkey
  FOREIGN KEY (organization_id) REFERENCES "organization" ("id");

-- =====================================================================
-- HELPER: app_current_org_id() — the organization of the currently bound
-- tenant. Fail-closed: with no tenant bound, app_current_tenant_id() is NULL,
-- the lookup matches no row, and the org policies below deny. Must be defined
-- after tenant.organization_id exists and after app_current_tenant_id().
-- =====================================================================

CREATE OR REPLACE FUNCTION app_current_org_id() RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT organization_id FROM tenant WHERE id = app_current_tenant_id()
$$;

-- =====================================================================
-- HARDEN org-plugin RLS: drop the permissive USING(true) policies and key them
-- to the current tenant's org (organization on id; member/invitation on
-- organization_id), with the platform-admin cross-org bypass. Privileged/web
-- writes bypass these by table ownership.
-- =====================================================================

DROP POLICY IF EXISTS "organization_all" ON "organization";
CREATE POLICY "organization_isolation" ON "organization" FOR ALL
  USING (id = app_current_org_id() OR app_is_platform_admin())
  WITH CHECK (id = app_current_org_id() OR app_is_platform_admin());

DROP POLICY IF EXISTS "member_all" ON "member";
CREATE POLICY "member_isolation" ON "member" FOR ALL
  USING (organization_id = app_current_org_id() OR app_is_platform_admin())
  WITH CHECK (organization_id = app_current_org_id() OR app_is_platform_admin());

DROP POLICY IF EXISTS "invitation_all" ON "invitation";
CREATE POLICY "invitation_isolation" ON "invitation" FOR ALL
  USING (organization_id = app_current_org_id() OR app_is_platform_admin())
  WITH CHECK (organization_id = app_current_org_id() OR app_is_platform_admin());
