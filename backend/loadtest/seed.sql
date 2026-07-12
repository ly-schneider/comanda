-- Load test seed — scoped to ONE tenant, resolved from its verified public host.
-- Idempotent: safe to re-run (refreshes session expiry, re-attaches devices).
--
-- Run against a PRIVILEGED (table-owner) connection — it writes across tenant
-- tables with an explicit tenant_id, so it must bypass RLS.
--
-- USAGE
--   psql "$DATABASE_URL" -f loadtest/seed.sql
--   psql "$DATABASE_URL" -v target_host=other.staging.comanda.leys.ch -f loadtest/seed.sql
--
-- The tenant is looked up by tenant_domain.hostname (the same source the backend
-- routes on), so every row lands in exactly the tenant that host serves. If no
-- verified domain matches, the seed aborts without touching anything.

\set ON_ERROR_STOP on

\if :{?target_host}
\else
  \set target_host 'load-test.staging.comanda.leys.ch'
\endif

-- psql does not interpolate :vars inside the dollar-quoted DO block below, so
-- hand the host to the session as a GUC (interpolated here, in plain SQL) and
-- read it back with current_setting() inside the block.
SELECT set_config('loadtest.host', :'target_host', false);

-- Match backend/internal/id: seeded product ids must pass nanoid.Valid(), which
-- rejects 0/O/o/I/i/J/j/L/l. Generate ids from the same alphabet.
CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

DO $seed$
DECLARE
  v_host   text := lower(current_setting('loadtest.host', true));
  v_tenant text;
  v_org    text;
  v_dev    text;
  v_cat    text;
  v_prod   text;
  i        int;
BEGIN
  SELECT t.id, t.organization_id
    INTO v_tenant, v_org
  FROM tenant t
  JOIN tenant_domain d ON d.tenant_id = t.id
  WHERE d.hostname = v_host AND d.verified_at IS NOT NULL;

  IF v_tenant IS NULL THEN
    RAISE EXCEPTION 'No verified tenant for host % — refusing to seed', v_host;
  END IF;

  RAISE NOTICE 'Seeding loadtest data into tenant % (host %, org %)', v_tenant, v_host, v_org;

  -- ── Customers (auth-plane tables are global, not tenant-scoped) ──
  FOR i IN 1..20 LOOP
    INSERT INTO "user" (id, email, name, email_verified)
    VALUES (
      'loadtest_user_customer_' || lpad(i::text, 3, '0'),
      'loadtest+customer-' || lpad(i::text, 3, '0') || '@loadtest.local',
      'Load Customer ' || i, TRUE)
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO session (id, user_id, token, expires_at, updated_at, active_organization_id)
    VALUES (
      'loadtest_sess_customer_' || lpad(i::text, 3, '0'),
      'loadtest_user_customer_' || lpad(i::text, 3, '0'),
      'loadtest_customer_session_' || lpad(i::text, 3, '0'),
      NOW() + INTERVAL '30 days', NOW(), v_org)
    ON CONFLICT (token) DO UPDATE
      SET expires_at = EXCLUDED.expires_at, updated_at = EXCLUDED.updated_at,
          active_organization_id = EXCLUDED.active_organization_id;
  END LOOP;

  -- ── Admins + org membership (member.role = staff → TierOps) ──
  FOR i IN 1..3 LOOP
    INSERT INTO "user" (id, email, name, email_verified)
    VALUES (
      'loadtest_user_admin_' || lpad(i::text, 3, '0'),
      'loadtest+admin-' || lpad(i::text, 3, '0') || '@loadtest.local',
      'Load Admin ' || i, TRUE)
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO session (id, user_id, token, expires_at, updated_at, active_organization_id)
    VALUES (
      'loadtest_sess_admin_' || lpad(i::text, 3, '0'),
      'loadtest_user_admin_' || lpad(i::text, 3, '0'),
      'loadtest_admin_session_' || lpad(i::text, 3, '0'),
      NOW() + INTERVAL '30 days', NOW(), v_org)
    ON CONFLICT (token) DO UPDATE
      SET expires_at = EXCLUDED.expires_at, updated_at = EXCLUDED.updated_at,
          active_organization_id = EXCLUDED.active_organization_id;

    IF v_org IS NOT NULL THEN
      INSERT INTO member (id, organization_id, user_id, role, created_at)
      VALUES (
        'loadtest_member_' || lpad(i::text, 3, '0'), v_org,
        'loadtest_user_admin_' || lpad(i::text, 3, '0'), 'staff', NOW())
      ON CONFLICT (id) DO NOTHING;
    END IF;
  END LOOP;

  -- ── Catalog: a category + 10 simple products with stock, so there is
  --    something to order. Only when the tenant has NO active simple product
  --    already — never pollutes a tenant that has a real catalog. ──
  IF NOT EXISTS (
    SELECT 1 FROM product
    WHERE tenant_id = v_tenant AND type = 'simple' AND is_active = TRUE
  ) THEN
    v_cat := pg_temp.nanoid();
    INSERT INTO category (id, tenant_id, name, is_active, position)
    VALUES (v_cat, v_tenant, 'Load Test', TRUE, 0);

    FOR i IN 1..10 LOOP
      v_prod := pg_temp.nanoid();
      INSERT INTO product (id, tenant_id, category_id, type, name, price_cents, is_active)
      VALUES (
        v_prod, v_tenant, v_cat, 'simple',
        'Load Item ' || lpad(i::text, 2, '0'), 300 + i * 50, TRUE);

      INSERT INTO inventory_ledger (id, tenant_id, product_id, delta, reason, created_by)
      VALUES (pg_temp.nanoid(), v_tenant, v_prod, 100000, 'manual_adjust', 'loadtest_user_admin_001');
    END LOOP;

    RAISE NOTICE 'Seeded loadtest catalog (10 simple products) into empty tenant %.', v_tenant;
  END IF;

  -- ── POS: two approved devices + bindings. device_id must resolve or the
  --    cash-payment order_payment.device_id FK (→ device) fails. ──
  FOR i IN 1..2 LOOP
    v_dev := 'ld_posdev_0' || i;
    INSERT INTO device (id, tenant_id, name, device_key, type, status, decided_at)
    VALUES (v_dev, v_tenant, 'Load POS ' || i, 'loadtest-pos-0' || i, 'POS', 'approved', NOW())
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO device_binding (id, tenant_id, device_type, token_hash, name, created_by_user_id, device_id)
    VALUES (
      'ld_pos_bnd0' || i, v_tenant, 'POS',
      encode(sha256(('loadtest_pos_token_' || lpad(i::text, 3, '0'))::bytea), 'hex'),
      'Load POS ' || i, 'loadtest_user_admin_001', v_dev)
    ON CONFLICT (token_hash) DO UPDATE
      SET revoked_at = NULL, device_id = EXCLUDED.device_id, last_seen_at = NOW();

    INSERT INTO session (id, user_id, token, expires_at, updated_at)
    VALUES (
      'loadtest_sess_pos_' || lpad(i::text, 3, '0'),
      'loadtest_user_admin_001',
      'loadtest_pos_token_' || lpad(i::text, 3, '0'),
      NOW() + INTERVAL '30 days', NOW())
    ON CONFLICT (token) DO UPDATE
      SET expires_at = EXCLUDED.expires_at, updated_at = EXCLUDED.updated_at;
  END LOOP;

  -- ── Station: one approved device with every active simple product assigned,
  --    plus three bindings. Without device_product the redeem scenario finds no
  --    matches and reports fast-but-empty responses. ──
  v_dev := 'ld_stndev_01';
  INSERT INTO device (id, tenant_id, name, device_key, type, status, decided_at)
  VALUES (v_dev, v_tenant, 'Load Station', 'loadtest-stn-01', 'STATION', 'approved', NOW())
  ON CONFLICT (id) DO NOTHING;

  INSERT INTO device_product (tenant_id, device_id, product_id)
  SELECT v_tenant, v_dev, p.id
  FROM product p
  WHERE p.tenant_id = v_tenant AND p.type = 'simple' AND p.is_active = TRUE
  ON CONFLICT (device_id, product_id) DO NOTHING;

  FOR i IN 1..3 LOOP
    INSERT INTO device_binding (id, tenant_id, device_type, token_hash, name, created_by_user_id, station_id)
    VALUES (
      'ld_stn_bnd0' || i, v_tenant, 'STATION',
      encode(sha256(('loadtest_station_token_' || lpad(i::text, 3, '0'))::bytea), 'hex'),
      'Load Station ' || i, 'loadtest_user_admin_001', v_dev)
    ON CONFLICT (token_hash) DO UPDATE
      SET revoked_at = NULL, station_id = EXCLUDED.station_id, last_seen_at = NOW();

    INSERT INTO session (id, user_id, token, expires_at, updated_at)
    VALUES (
      'loadtest_sess_station_' || lpad(i::text, 3, '0'),
      'loadtest_user_admin_001',
      'loadtest_station_token_' || lpad(i::text, 3, '0'),
      NOW() + INTERVAL '30 days', NOW())
    ON CONFLICT (token) DO UPDATE
      SET expires_at = EXCLUDED.expires_at, updated_at = EXCLUDED.updated_at;
  END LOOP;

  -- Make sure the storefront accepts orders (RequireEnabled / "Aktuell geschlossen").
  UPDATE settings SET system_enabled = TRUE WHERE id = v_tenant;

  RAISE NOTICE 'Seed complete for tenant %.', v_tenant;
END
$seed$;

\echo ''
\echo 'Seed summary (privileged view):'
SELECT 'customers'        AS kind, COUNT(*) FROM "user"          WHERE id    LIKE 'loadtest_user_customer_%'
UNION ALL
SELECT 'admins'           AS kind, COUNT(*) FROM "user"          WHERE id    LIKE 'loadtest_user_admin_%'
UNION ALL
SELECT 'sessions'         AS kind, COUNT(*) FROM session         WHERE token LIKE 'loadtest_%'
UNION ALL
SELECT 'devices'          AS kind, COUNT(*) FROM device          WHERE id    LIKE 'ld_%dev_0%'
UNION ALL
SELECT 'device bindings'  AS kind, COUNT(*) FROM device_binding  WHERE id    LIKE 'ld_%bnd0%' AND revoked_at IS NULL
UNION ALL
SELECT 'station products' AS kind, COUNT(*) FROM device_product  WHERE device_id = 'ld_stndev_01'
UNION ALL
SELECT 'active simple products' AS kind, COUNT(*) FROM product    WHERE type = 'simple' AND is_active = TRUE
  AND tenant_id = (SELECT tenant_id FROM tenant_domain WHERE hostname = current_setting('loadtest.host', true) AND verified_at IS NOT NULL);
