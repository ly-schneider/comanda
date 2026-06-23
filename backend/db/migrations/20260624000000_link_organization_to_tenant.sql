-- organization_id is Better-Auth-shaped TEXT. Nullable with a unique index so a
-- tenant mid-create may be unlinked (many NULLs) but at most one per organization.
ALTER TABLE tenant ADD COLUMN organization_id TEXT;
CREATE UNIQUE INDEX tenant_organization_id_key ON tenant (organization_id);
ALTER TABLE tenant
  ADD CONSTRAINT tenant_organization_id_fkey
  FOREIGN KEY (organization_id) REFERENCES "organization" ("id");

-- STABLE, not IMMUTABLE: it reads app_current_tenant_id() (a per-statement GUC).
-- Fail-closed — with no tenant bound the lookup matches no row and the org
-- policies below deny.
CREATE OR REPLACE FUNCTION app_current_org_id() RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT organization_id FROM tenant WHERE id = app_current_tenant_id()
$$;

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
