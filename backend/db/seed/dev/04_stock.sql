-- Dev-only: sizes the morning delivery to what the weekend sold, plus headroom.
-- Run after 02_sales.sql or products end up with negative stock.

\set ON_ERROR_STOP on

SET TIME ZONE 'Europe/Zurich';

UPDATE inventory_ledger il
   SET delta = sold.needed
  FROM (
    SELECT product_id, variant_id, ceil(sum(-delta) * 1.3)::int AS needed
      FROM inventory_ledger
     WHERE tenant_id = :'seed_tenant'
       AND reason = 'sale'
     GROUP BY product_id, variant_id
  ) AS sold
 WHERE il.tenant_id = :'seed_tenant'
   AND il.reason = 'opening_balance'
   AND il.created_at = timestamptz '2026-08-14 08:00:00+02'
   AND il.product_id = sold.product_id
   AND il.variant_id IS NOT DISTINCT FROM sold.variant_id;

-- What is left standing on Sunday night.
SELECT p.name, v.value AS variante, sum(il.delta) AS bestand
  FROM inventory_ledger il
  JOIN product p ON p.id = il.product_id
  LEFT JOIN product_variant v ON v.id = il.variant_id
 WHERE il.tenant_id = :'seed_tenant'
 GROUP BY p.name, v.value
 ORDER BY p.name, v.value;
