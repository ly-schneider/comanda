-- 20260603000000_tenant_domain_and_platform_admin.sql
-- P2 (LEY-311): host-based tenant resolution + platform-admin cross-tenant reads.
--
--   * tenant_domain table: maps a public hostname -> tenant. The resolver reads
--     it via the privileged (RLS-bypassing) client before app.tenant_id is set.
--   * app.is_platform_admin GUC + app_is_platform_admin() helper.
--   * user.is_platform_admin column (admin-set out of band; the web slice
--     declares it as a Better Auth additionalField — additive, the adapter
--     won't touch it).
--   * every tenant-scoped RLS policy rewritten to add an OR app_is_platform_admin()
--     bypass on both USING and WITH CHECK.
--
-- RLS stays the isolation boundary. Platform-admin cross-tenant access requires
-- TWO independent gates: the verified user.is_platform_admin column (which sets
-- the GUC) AND the GUC itself. The Ent-layer guard is defense-in-depth only.

-- =====================================================================
-- HELPER: app_is_platform_admin() — read the GUC, default false.
-- =====================================================================

CREATE OR REPLACE FUNCTION app_is_platform_admin() RETURNS boolean
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT current_setting('app.is_platform_admin', true) = 'true'
$$;

-- =====================================================================
-- USER: platform-admin flag.
-- =====================================================================

ALTER TABLE "user" ADD COLUMN is_platform_admin boolean NOT NULL DEFAULT false;

-- =====================================================================
-- tenant_domain: hostname -> tenant. Tenant-scoped (carries tenant_id) so the
-- app role and the Ent guards only ever see their own rows; the resolver uses
-- the privileged client (ownership bypass) to read it before a tenant is set.
-- =====================================================================

CREATE TYPE tenant_domain_cert_status AS ENUM ('none', 'pending', 'active', 'failed');

CREATE TABLE tenant_domain (
    id VARCHAR(12) PRIMARY KEY,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    hostname TEXT NOT NULL CHECK (hostname = lower(hostname)),
    is_primary BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ,
    cert_status tenant_domain_cert_status NOT NULL DEFAULT 'none',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Hostnames are stored lowercase (enforced by the CHECK above), so the resolver
-- can match with a plain equality lookup on the lowercased request host. A flat
-- unique index keeps the DB consistent with the Ent schema's unique declaration.
CREATE UNIQUE INDEX tenant_domain_hostname_key ON tenant_domain (hostname);
CREATE INDEX tenant_domain_tenant_id_idx ON tenant_domain (tenant_id);
CREATE UNIQUE INDEX tenant_domain_one_primary ON tenant_domain (tenant_id) WHERE is_primary;

ALTER TABLE tenant_domain ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_domain_isolation ON tenant_domain FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_domain TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON tenant_domain TO comanda_privileged;
  END IF;
END$$;

-- Seed the bootstrap tenant's hosts so local dev + tests keep resolving.
INSERT INTO tenant_domain (id, tenant_id, hostname, is_primary) VALUES
  ('tdmn_seed__1', 'tenant_seed1', 'localhost', true),
  ('tdmn_seed__2', 'tenant_seed1', '127.0.0.1', false),
  ('tdmn_seed__3', 'tenant_seed1', 'default.comanda.leys.ch', false);

-- =====================================================================
-- REWRITE tenant-scoped RLS policies to add the platform-admin bypass.
-- P1 created <table>_isolation policies with USING (tenant_id = app_current_tenant_id()).
-- Drop + recreate each with an OR app_is_platform_admin() on USING and WITH CHECK.
-- =====================================================================

DO $$
DECLARE
  t text;
  scoped text[] := ARRAY[
    'product','category','jeton','menu_slot','menu_slot_option','order',
    'order_line','order_payment','order_line_redemption','inventory_ledger',
    'club100_redemption','club100_free_product','settings','idempotency',
    'device','device_binding','device_product','admin_invite',
    'volunteer_campaign','volunteer_campaign_product','volunteer_redemption'
  ];
BEGIN
  FOREACH t IN ARRAY scoped LOOP
    EXECUTE format('DROP POLICY IF EXISTS %I ON %I', t || '_isolation', t);
    EXECUTE format(
      'CREATE POLICY %I ON %I FOR ALL '
      'USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin()) '
      'WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin())',
      t || '_isolation', t
    );
  END LOOP;
END$$;

-- tenant root: keyed on id, not tenant_id.
DROP POLICY IF EXISTS tenant_isolation ON tenant;
CREATE POLICY tenant_isolation ON tenant FOR ALL
  USING (id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (id = app_current_tenant_id() OR app_is_platform_admin());
