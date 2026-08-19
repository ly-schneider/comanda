-- Dev-only: rounds the demo catalog out so a sales export has every column to
-- show — deposits, variants, and a "3 für 2" combo modelled the way BlessThun
-- actually built theirs (one flat product, not a bundle).
--
-- Usage: psql "$DATABASE_URL" -v seed_tenant=<tenant id> -f 01_catalog_extras.sql

\set ON_ERROR_STOP on

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

-- Returnable cups: every cold drink carries a deposit, the Capri Sun pouch does not.
UPDATE product SET deposit_cents = 200
WHERE tenant_id = :'seed_tenant'
  AND id IN ('prd________1', 'prd________2', 'prd________3', 'prd________4', 'prd________6');

INSERT INTO category (id, tenant_id, name, position, is_active, parent_id)
VALUES ('cat_dev_mrch', :'seed_tenant', 'Merch', 5, true, NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO product (id, tenant_id, category_id, type, name, price_cents, deposit_cents, is_active, variant_label)
VALUES
  ('prd_dev_shrt', :'seed_tenant', 'cat_dev_mrch', 'simple', 'BlessThun Shirt', 3000, 0, true, 'Grösse'),
  -- Deliberately a single flat product: the report can then only ever see "one
  -- pack sold", never the three shirts inside it.
  ('prd_dev_3f2_', :'seed_tenant', 'cat_dev_mrch', 'simple', 'Shirt 3 für 2', 6000, 0, true, NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO product_variant (id, tenant_id, product_id, value, price_adjustment_cents, is_active, sequence)
VALUES
  ('var_dev_shrs', :'seed_tenant', 'prd_dev_shrt', 'S', 0, true, 0),
  ('var_dev_shrm', :'seed_tenant', 'prd_dev_shrt', 'M', 0, true, 1),
  ('var_dev_shrl', :'seed_tenant', 'prd_dev_shrt', 'L', 0, true, 2),
  ('var_dev_shrx', :'seed_tenant', 'prd_dev_shrt', 'XL', 200, true, 3)
ON CONFLICT (id) DO NOTHING;

-- The delivery on the morning of the first event day. Dated inside the reported
-- range on purpose, so it shows up as a movement and not as opening stock.
INSERT INTO inventory_ledger (id, tenant_id, product_id, variant_id, delta, reason, created_by, created_at)
SELECT pg_temp.nanoid(), :'seed_tenant', v.product_id, v.variant_id, v.delta, 'opening_balance', 'kBHQDv91PRkx',
       timestamptz '2026-08-14 08:00:00+02'
FROM (VALUES
  ('prd________1', NULL, 700), ('prd________2', NULL, 400), ('prd________3', NULL, 600),
  ('prd________4', NULL, 500), ('prd________6', NULL, 350), ('prd________7', NULL, 250),
  ('prd_______11', NULL, 700), ('prd_______12', NULL, 300), ('prd_______21', NULL, 800),
  ('prd_dev_shrt', 'var_dev_shrs', 40), ('prd_dev_shrt', 'var_dev_shrm', 70),
  ('prd_dev_shrt', 'var_dev_shrl', 70), ('prd_dev_shrt', 'var_dev_shrx', 30),
  ('prd_dev_3f2_', NULL, 60)
) AS v(product_id, variant_id, delta)
WHERE NOT EXISTS (
  SELECT 1 FROM inventory_ledger il
  WHERE il.tenant_id = :'seed_tenant'
    AND il.created_at = timestamptz '2026-08-14 08:00:00+02'
);
