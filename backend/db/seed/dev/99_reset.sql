-- Dev-only: removes everything the 01–04 fixtures wrote for a tenant. Keyed on
-- the seeded event window, so anything you created outside 14.–16.08.2026 stays.
--
-- Usage: psql "$DATABASE_URL" -v seed_tenant=<tenant id> -f 99_reset.sql

\set ON_ERROR_STOP on

BEGIN;

SET LOCAL TIME ZONE 'Europe/Zurich';

CREATE TEMP TABLE seed_orders ON COMMIT DROP AS
SELECT id FROM "order"
 WHERE tenant_id = :'seed_tenant'
   AND created_at >= timestamptz '2026-08-14 00:00:00+02'
   AND created_at <  timestamptz '2026-08-17 00:00:00+02';

-- Refunds first: refund.order_id is NO ACTION, so it would block the delete.
DELETE FROM refund_line WHERE refund_id IN (
  SELECT r.id FROM refund r JOIN seed_orders s ON s.id = r.order_id
);
DELETE FROM refund WHERE order_id IN (SELECT id FROM seed_orders);

-- Ledger rows would only be orphaned (order_id is ON DELETE SET NULL) and would
-- then read as expired reservations, so they go explicitly: the ones tied to
-- seeded orders, the seeded delivery, and whatever the pending sweeper already
-- detached.
DELETE FROM inventory_ledger
 WHERE tenant_id = :'seed_tenant'
   AND (
     order_id IN (SELECT id FROM seed_orders)
     OR (reason = 'opening_balance' AND created_at = timestamptz '2026-08-14 08:00:00+02')
     OR (order_id IS NULL AND reason IN ('sale', 'correction')
         AND created_at >= timestamptz '2026-08-14 00:00:00+02')
   );

-- Lines and payments cascade with the order.
DELETE FROM "order" WHERE id IN (SELECT id FROM seed_orders);

DELETE FROM device WHERE tenant_id = :'seed_tenant' AND device_key LIKE 'dev-seed-kasse-%';

-- Catalog extras from 01_catalog_extras.sql.
DELETE FROM product_variant WHERE tenant_id = :'seed_tenant' AND product_id = 'prd_dev_shrt';
DELETE FROM product WHERE tenant_id = :'seed_tenant' AND id IN ('prd_dev_shrt', 'prd_dev_3f2_');
DELETE FROM category WHERE tenant_id = :'seed_tenant' AND id = 'cat_dev_mrch';
UPDATE product SET deposit_cents = 0
 WHERE tenant_id = :'seed_tenant'
   AND id IN ('prd________1', 'prd________2', 'prd________3', 'prd________4', 'prd________6');

COMMIT;
