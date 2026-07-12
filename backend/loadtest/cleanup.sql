-- Remove everything loadtest/seed.sql and loadtest/stock.sql created, plus the
-- orders generated during a run — all scoped to the tenant resolved from its
-- verified public host. Other tenants are never touched. Run privileged.
--
-- USAGE
--   psql "$DATABASE_URL" -f loadtest/cleanup.sql
--   psql "$DATABASE_URL" -v target_host=other.staging.comanda.leys.ch -f loadtest/cleanup.sql

\set ON_ERROR_STOP on

\if :{?target_host}
\else
  \set target_host 'load-test.staging.comanda.leys.ch'
\endif

-- Hand the host to the session as a GUC; psql doesn't interpolate :vars inside
-- the dollar-quoted DO block.
SELECT set_config('loadtest.host', :'target_host', false);

DO $cleanup$
DECLARE
  v_host   text := lower(current_setting('loadtest.host', true));
  v_tenant text;
BEGIN
  SELECT t.id INTO v_tenant
  FROM tenant t
  JOIN tenant_domain d ON d.tenant_id = t.id
  WHERE d.hostname = v_host AND d.verified_at IS NOT NULL;

  IF v_tenant IS NULL THEN
    RAISE EXCEPTION 'No verified tenant for host % — refusing to clean up', v_host;
  END IF;

  -- Orders: customer orders carry a loadtest contact email; POS/station orders
  -- have none but were paid by our loadtest POS devices. Deleting an order
  -- cascades order_payment / order_line / order_line_redemption.
  DELETE FROM "order" o
  WHERE o.tenant_id = v_tenant
    AND (
      o.contact_email LIKE 'loadtest+%@loadtest.local'
      OR EXISTS (
        SELECT 1 FROM order_payment op
        WHERE op.order_id = o.id AND op.device_id IN ('ld_posdev_01', 'ld_posdev_02')
      )
    );

  DELETE FROM inventory_ledger
  WHERE tenant_id = v_tenant AND created_by = 'loadtest_user_admin_001' AND reason = 'manual_adjust';

  -- device_product FKs device with ON DELETE RESTRICT, so clear it before device.
  DELETE FROM device_product WHERE tenant_id = v_tenant AND device_id LIKE 'ld_%dev_0%';
  DELETE FROM device_binding WHERE tenant_id = v_tenant AND id LIKE 'ld_%bnd0%';
  DELETE FROM device         WHERE tenant_id = v_tenant AND id LIKE 'ld_%dev_0%';

  -- Seeded catalog (ids are random nanoids, so match by the seeded names).
  -- Orders (and their order_lines) are gone above, so the order_line→product
  -- RESTRICT is clear; delete products before the category they FK
  -- (category_id RESTRICT). Product delete cascades any remaining
  -- inventory_ledger / device_product rows.
  DELETE FROM product  WHERE tenant_id = v_tenant AND name LIKE 'Load Item %';
  DELETE FROM category WHERE tenant_id = v_tenant AND name = 'Load Test';

  DELETE FROM member  WHERE id    LIKE 'loadtest_member_%';
  DELETE FROM session WHERE token LIKE 'loadtest_%';
  DELETE FROM "user"  WHERE id    LIKE 'loadtest_%';

  RAISE NOTICE 'Loadtest data removed from tenant %.', v_tenant;
END
$cleanup$;
