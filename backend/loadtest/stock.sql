-- Bump inventory for every active simple product so a baseline loadtest run
-- can't deplete stock. Tagged with created_by = 'loadtest_user_admin_001' so
-- cleanup.sql can remove just these entries without touching real balances.
--
-- USAGE
--   psql "$DATABASE_URL" -f loadtest/stock.sql

\set ON_ERROR_STOP on

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZ_abcdefghkmnpqrstuvwxyz-',
                           (floor(random()*55))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

BEGIN;

INSERT INTO inventory_ledger (id, tenant_id, product_id, delta, reason, created_by)
SELECT pg_temp.nanoid(), p.tenant_id, p.id, 50000, 'manual_adjust', 'loadtest_user_admin_001'
FROM product p
WHERE type = 'simple' AND is_active = TRUE;

COMMIT;

\echo ''
\echo 'Stock summary after bump:'
SELECT
  p.name,
  COALESCE(SUM(il.delta), 0) AS stock
FROM product p
LEFT JOIN inventory_ledger il ON il.product_id = p.id
WHERE p.type = 'simple' AND p.is_active = TRUE
GROUP BY p.id, p.name
ORDER BY p.name;
