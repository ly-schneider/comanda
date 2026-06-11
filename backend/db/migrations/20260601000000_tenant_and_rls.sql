-- 20260601000000_tenant_and_rls.sql
-- P1 (LEY-310): introduce shared-schema multi-tenancy.
--
--   * tenant root table.
--   * tenant_id (+ index + FK) on every tenant-scoped table; nullable -> backfill
--     to a bootstrap tenant -> SET NOT NULL, so this is safe on a populated DB.
--   * per-tenant re-scoping of unique constraints that would otherwise collide
--     across tenants.
--   * app_current_tenant_id() helper (fail-closed) + RLS policies on every
--     scoped table, permissive policies on the global auth tables.
--
-- RLS is the isolation boundary. The owner role (comanda_privileged) bypasses
-- RLS via table ownership; the app role (comanda_app) is filtered by these
-- policies on the app.tenant_id GUC set per request. No FORCE ROW LEVEL
-- SECURITY — the owner must bypass for migrations and cron.

-- =====================================================================
-- TENANT ROOT
-- =====================================================================

CREATE TYPE tenant_status AS ENUM ('active', 'suspended');

CREATE TABLE tenant (
    id VARCHAR(12) PRIMARY KEY,
    slug VARCHAR(63) NOT NULL,
    name VARCHAR(100) NOT NULL,
    status tenant_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX tenant_slug_key ON tenant (slug);

-- Bootstrap tenant: existing single-tenant data is migrated under it. Its slug
-- matches BOOTSTRAP_TENANT_SLUG so the P1 stopgap resolver can find it.
INSERT INTO tenant (id, slug, name)
VALUES ('tenant_seed1', 'default', 'Default Tenant');

-- =====================================================================
-- tenant_id COLUMN + INDEX + FK + BACKFILL + NOT NULL, per scoped table.
-- =====================================================================

DO $$
DECLARE
  t text;
  bootstrap_id constant text := 'tenant_seed1';
  scoped text[] := ARRAY[
    'product','category','jeton','menu_slot','menu_slot_option','order',
    'order_line','order_payment','order_line_redemption','inventory_ledger',
    'club100_redemption','club100_free_product','settings','idempotency',
    'device','device_binding','device_product','admin_invite',
    'volunteer_campaign','volunteer_campaign_product','volunteer_redemption'
  ];
BEGIN
  FOREACH t IN ARRAY scoped LOOP
    EXECUTE format('ALTER TABLE %I ADD COLUMN tenant_id varchar(12)', t);
    EXECUTE format('UPDATE %I SET tenant_id = %L WHERE tenant_id IS NULL', t, bootstrap_id);
    EXECUTE format('ALTER TABLE %I ALTER COLUMN tenant_id SET NOT NULL', t);
    EXECUTE format(
      'ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (tenant_id) REFERENCES tenant (id)',
      t, t || '_tenant_id_fkey'
    );
    EXECUTE format('CREATE INDEX %I ON %I (tenant_id)', t || '_tenant_id_idx', t);
  END LOOP;
END$$;

-- =====================================================================
-- PER-TENANT RE-SCOPING OF UNIQUE CONSTRAINTS
-- A globally-unique natural key would let one tenant's value block another's.
-- Cryptographic secrets (token_hash, claim_token) stay globally unique on
-- purpose: they are looked up by secret and must not collide.
-- =====================================================================

-- idempotency (scope,key) -> (tenant_id, scope, key). The baseline left both a
-- UNIQUE constraint (idempotency_scope_key_key) and a redundant unique index
-- (idempotency_scope_key_unique); drop the constraint first, then the index.
ALTER TABLE idempotency DROP CONSTRAINT IF EXISTS idempotency_scope_key_key;
DROP INDEX IF EXISTS idempotency_scope_key_unique;
CREATE UNIQUE INDEX idempotency_tenant_scope_key_unique
  ON idempotency (tenant_id, scope, key);

-- device.device_key -> (tenant_id, device_key)
DROP INDEX IF EXISTS idx_device_device_key;
CREATE UNIQUE INDEX idx_device_tenant_device_key
  ON device (tenant_id, device_key);

-- device.pairing_code (partial) -> (tenant_id, pairing_code)
DROP INDEX IF EXISTS idx_device_pairing_code;
CREATE UNIQUE INDEX idx_device_tenant_pairing_code
  ON device (tenant_id, pairing_code) WHERE pairing_code IS NOT NULL;

-- order.payment_attempt_id (partial, pending) -> tenant-scoped
DROP INDEX IF EXISTS idx_order_payment_attempt_pending;
CREATE UNIQUE INDEX idx_order_tenant_payment_attempt_pending
  ON "order" (tenant_id, payment_attempt_id)
  WHERE status = 'pending' AND payment_attempt_id IS NOT NULL;

-- volunteer_redemption (campaign_id, idempotency_key) -> tenant-scoped
DROP INDEX IF EXISTS idx_volunteer_redemption_campaign_idempotency;
CREATE UNIQUE INDEX idx_volunteer_redemption_tenant_campaign_idempotency
  ON volunteer_redemption (tenant_id, campaign_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

-- =====================================================================
-- SETTINGS: per-tenant singleton.
-- The pre-tenant schema had a single row id='default'. Repoint that row (and
-- the club100_free_product rows that FK to it) to the bootstrap tenant's id so
-- the primary key never collides across tenants while RLS isolates visibility.
-- =====================================================================

UPDATE club100_free_product SET settings_id = 'tenant_seed1'
  WHERE settings_id = 'default';
UPDATE settings SET id = 'tenant_seed1'
  WHERE id = 'default';

-- =====================================================================
-- HELPER: read the request tenant GUC, fail-closed.
-- =====================================================================

CREATE OR REPLACE FUNCTION app_current_tenant_id() RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT NULLIF(current_setting('app.tenant_id', true), '')
$$;

-- =====================================================================
-- RLS POLICIES — scoped tables. FOR ALL with USING + WITH CHECK on tenant_id.
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
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY %I ON %I FOR ALL USING (tenant_id = app_current_tenant_id()) WITH CHECK (tenant_id = app_current_tenant_id())',
      t || '_isolation', t
    );
  END LOOP;
END$$;

-- =====================================================================
-- RLS POLICIES — tenant root + global auth tables.
-- tenant: a tenant can see/modify only its own row (privileged bypass handles
-- provisioning). Auth tables: permissive — login happens before a tenant is
-- known; control is the privileged-role split, mirroring iglesio.
-- =====================================================================

ALTER TABLE tenant ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant FOR ALL
  USING (id = app_current_tenant_id())
  WITH CHECK (id = app_current_tenant_id());

DO $$
DECLARE
  t text;
  auth_tables text[] := ARRAY['user','session','account','verification','jwks','api_key'];
BEGIN
  FOREACH t IN ARRAY auth_tables LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY %I ON %I FOR ALL USING (true) WITH CHECK (true)',
      t || '_all', t
    );
  END LOOP;
END$$;

-- =====================================================================
-- GRANTS for the app role on the new tenant table (other tables were granted
-- by bootstrap-rls-roles.sql / ALTER DEFAULT PRIVILEGES).
-- =====================================================================

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON tenant TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON tenant TO comanda_privileged;
  END IF;
END$$;
