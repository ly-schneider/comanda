-- Bump inventory for every active simple product in the load-test tenant so a
-- run can't deplete stock. Tagged created_by = 'loadtest_user_admin_001' so
-- cleanup.sql removes just these entries without touching real balances.
-- Scoped to the tenant resolved from its verified public host; other tenants
-- are never touched. Run privileged.
--
-- USAGE
--   psql "$DATABASE_URL" -f loadtest/stock.sql
--   psql "$DATABASE_URL" -v target_host=other.staging.comanda.leys.ch -f loadtest/stock.sql

\set ON_ERROR_STOP on

\if :{?target_host}
\else
  \set target_host 'load-test.staging.comanda.leys.ch'
\endif

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

-- Hand the host to the session as a GUC; psql doesn't interpolate :vars inside
-- the dollar-quoted DO block.
SELECT set_config('loadtest.host', :'target_host', false);

DO $stock$
DECLARE
  v_host   text := lower(current_setting('loadtest.host', true));
  v_tenant text;
  n        int;
BEGIN
  SELECT t.id INTO v_tenant
  FROM tenant t
  JOIN tenant_domain d ON d.tenant_id = t.id
  WHERE d.hostname = v_host AND d.verified_at IS NOT NULL;

  IF v_tenant IS NULL THEN
    RAISE EXCEPTION 'No verified tenant for host % — refusing to adjust stock', v_host;
  END IF;

  INSERT INTO inventory_ledger (id, tenant_id, product_id, delta, reason, created_by)
  SELECT pg_temp.nanoid(), v_tenant, p.id, 50000, 'manual_adjust', 'loadtest_user_admin_001'
  FROM product p
  WHERE p.tenant_id = v_tenant AND p.type = 'simple' AND p.is_active = TRUE;

  GET DIAGNOSTICS n = ROW_COUNT;
  RAISE NOTICE 'Bumped stock for % simple product(s) in tenant %.', n, v_tenant;
END
$stock$;
