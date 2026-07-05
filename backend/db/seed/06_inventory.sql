-- tenant_id has no DB default; default it to the bootstrap tenant for this seed.
ALTER TABLE inventory_ledger ALTER COLUMN tenant_id SET DEFAULT 'tenant_seed1';

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

-- Add opening balance for Getränke (50 each)
INSERT INTO inventory_ledger (id, product_id, delta, reason)
SELECT pg_temp.nanoid(), id, 50, 'opening_balance'
FROM product
WHERE category_id = 'cat________1'
  AND NOT EXISTS (
    SELECT 1 FROM inventory_ledger il
    WHERE il.product_id = product.id AND il.reason = 'opening_balance'
  );

-- Add opening balance for Essen (30 each)
INSERT INTO inventory_ledger (id, product_id, delta, reason)
SELECT pg_temp.nanoid(), id, 30, 'opening_balance'
FROM product
WHERE category_id = 'cat________2'
  AND NOT EXISTS (
    SELECT 1 FROM inventory_ledger il
    WHERE il.product_id = product.id AND il.reason = 'opening_balance'
  );

-- Add opening balance for Beilage (40 each)
INSERT INTO inventory_ledger (id, product_id, delta, reason)
SELECT pg_temp.nanoid(), id, 40, 'opening_balance'
FROM product
WHERE category_id = 'cat________3'
  AND NOT EXISTS (
    SELECT 1 FROM inventory_ledger il
    WHERE il.product_id = product.id AND il.reason = 'opening_balance'
  );

ALTER TABLE inventory_ledger ALTER COLUMN tenant_id DROP DEFAULT;
