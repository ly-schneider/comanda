-- Dev-only: layers the money that came back onto the seeded event — line
-- refunds with their released deposit, deposit returns, full cancellations,
-- admin cancellations without a payout, and a couple of arbitrary refunds that
-- no article can be pinned on.
--
-- Usage: psql "$DATABASE_URL" -v seed_tenant=<tenant id> -f 03_refunds.sql

\set ON_ERROR_STOP on

BEGIN;

SET LOCAL TIME ZONE 'Europe/Zurich';
SELECT set_config('seed.tenant', :'seed_tenant', true);

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

CREATE TEMP TABLE seed_candidates ON COMMIT DROP AS
SELECT o.id, o.total_cents, o.created_at, o.payrexx_transaction_id,
       (o.payrexx_transaction_id IS NOT NULL) AS by_twint,
       row_number() OVER (ORDER BY random()) AS rn
  FROM "order" o
  JOIN order_payment p ON p.order_id = o.id
 WHERE o.tenant_id = current_setting('seed.tenant')
   AND o.status = 'paid'
   AND p.method IN ('CASH', 'CARD', 'TWINT')
   AND o.created_at >= timestamptz '2026-08-14 00:00:00+02'
   AND o.created_at <  timestamptz '2026-08-17 00:00:00+02';

DO $seed$
DECLARE
  v_tenant     text := current_setting('seed.tenant');
  v_actor      text := 'kBHQDv91PRkx';
  v_order      record;
  v_line       record;
  v_dep        record;
  v_refund_id  varchar(12);
  v_amount     bigint;
  v_at         timestamptz;
  v_method     text;
BEGIN
  -- 1. Line refunds: one item handed back, its deposit released with it.
  FOR v_order IN SELECT * FROM seed_candidates WHERE rn <= 40 LOOP
    SELECT l.id, l.product_id, l.variant_id, l.unit_price_cents, l.line_type::text AS line_type,
           p.type::text AS product_type
      INTO v_line
      FROM order_line l
      JOIN product p ON p.id = l.product_id
     WHERE l.order_id = v_order.id
       AND l.line_type IN ('simple', 'bundle')
       AND l.unit_price_cents > 0
     ORDER BY random() LIMIT 1;
    CONTINUE WHEN NOT FOUND;

    v_refund_id := pg_temp.nanoid();
    v_amount := v_line.unit_price_cents;
    v_at := v_order.created_at + make_interval(mins => floor(random() * 120 + 5)::int);
    v_method := CASE WHEN v_order.by_twint THEN 'twint' ELSE 'cash' END;

    SELECT id, unit_price_cents INTO v_dep
      FROM order_line
     WHERE order_id = v_order.id AND line_type = 'deposit' AND parent_line_id = v_line.id
     LIMIT 1;
    IF FOUND THEN
      v_amount := v_amount + v_dep.unit_price_cents;
    END IF;

    INSERT INTO refund (id, tenant_id, order_id, amount_cents, kind, method, reason, actor_type, actor_id,
                        payrexx_transaction_id, status, created_at)
    VALUES (v_refund_id, v_tenant, v_order.id, v_amount, 'line', v_method::refund_method,
            (ARRAY['Falsch bestellt', 'Falsch ausgegeben', 'Getränk umgekippt', NULL])[floor(random()*4)+1],
            'operator', v_actor, v_order.payrexx_transaction_id, 'succeeded', v_at);

    INSERT INTO refund_line (id, tenant_id, refund_id, order_line_id, quantity, amount_cents)
    VALUES (pg_temp.nanoid(), v_tenant, v_refund_id, v_line.id, 1, v_line.unit_price_cents);
    IF v_dep.id IS NOT NULL THEN
      INSERT INTO refund_line (id, tenant_id, refund_id, order_line_id, quantity, amount_cents)
      VALUES (pg_temp.nanoid(), v_tenant, v_refund_id, v_dep.id, 1, v_dep.unit_price_cents);
    END IF;

    -- Only a simple product comes back into stock. A refunded bundle line
    -- restores nothing today (its components are never expanded) — seeded that
    -- way on purpose so the data shows the real behaviour.
    IF v_line.product_type = 'simple' THEN
      INSERT INTO inventory_ledger (id, tenant_id, product_id, variant_id, delta, reason, order_id,
                                    order_line_id, created_at)
      VALUES (pg_temp.nanoid(), v_tenant, v_line.product_id, v_line.variant_id, 1, 'refund', v_order.id,
              v_line.id, v_at);
    END IF;

    UPDATE "order"
       SET status = CASE WHEN v_amount >= total_cents THEN 'refunded'::order_status
                         ELSE 'partially_refunded'::order_status END,
           updated_at = v_at
     WHERE id = v_order.id;
  END LOOP;

  -- 2. Full cancellations with the money paid back.
  FOR v_order IN SELECT * FROM seed_candidates WHERE rn BETWEEN 41 AND 52 LOOP
    v_refund_id := pg_temp.nanoid();
    v_at := v_order.created_at + make_interval(mins => floor(random() * 60 + 5)::int);
    v_method := CASE WHEN v_order.by_twint THEN 'twint' ELSE 'cash' END;

    INSERT INTO refund (id, tenant_id, order_id, amount_cents, kind, method, reason, actor_type, actor_id,
                        payrexx_transaction_id, status, created_at)
    VALUES (v_refund_id, v_tenant, v_order.id, v_order.total_cents, 'line', v_method::refund_method,
            'Bestellung storniert', 'admin', v_actor, v_order.payrexx_transaction_id, 'succeeded', v_at);

    INSERT INTO refund_line (id, tenant_id, refund_id, order_line_id, quantity, amount_cents)
    SELECT pg_temp.nanoid(), v_tenant, v_refund_id, l.id, l.quantity, l.unit_price_cents * l.quantity
      FROM order_line l
     WHERE l.order_id = v_order.id;

    INSERT INTO inventory_ledger (id, tenant_id, product_id, variant_id, delta, reason, order_id,
                                  order_line_id, created_at)
    SELECT pg_temp.nanoid(), v_tenant, l.product_id, l.variant_id, l.quantity, 'refund', v_order.id, l.id, v_at
      FROM order_line l
      JOIN product p ON p.id = l.product_id
     WHERE l.order_id = v_order.id
       AND l.line_type IN ('simple', 'component')
       AND p.type = 'simple';

    UPDATE "order" SET status = 'cancelled', updated_at = v_at, qr_payload = NULL WHERE id = v_order.id;
  END LOOP;

  -- 3. Arbitrary goodwill refunds: real money out, no article to hang it on.
  FOR v_order IN SELECT * FROM seed_candidates WHERE rn BETWEEN 53 AND 55 LOOP
    v_refund_id := pg_temp.nanoid();
    v_at := v_order.created_at + make_interval(mins => floor(random() * 180 + 30)::int);
    v_amount := least(v_order.total_cents, (floor(random() * 8 + 2) * 100)::bigint);
    v_method := CASE WHEN v_order.by_twint THEN 'twint' ELSE 'cash' END;

    INSERT INTO refund (id, tenant_id, order_id, amount_cents, kind, method, reason, actor_type, actor_id,
                        payrexx_transaction_id, status, created_at)
    VALUES (v_refund_id, v_tenant, v_order.id, v_amount, 'arbitrary', v_method::refund_method,
            'Kulanz', 'admin', v_actor, v_order.payrexx_transaction_id, 'succeeded', v_at);

    UPDATE "order"
       SET status = CASE WHEN v_amount >= total_cents THEN 'refunded'::order_status
                         ELSE 'partially_refunded'::order_status END,
           updated_at = v_at
     WHERE id = v_order.id;
  END LOOP;

  -- 4. Deposit returns at the end of the evening: cups come back, no stock moves.
  FOR v_order IN
    SELECT c.* FROM seed_candidates c
     WHERE c.rn > 55
       AND EXISTS (SELECT 1 FROM order_line l WHERE l.order_id = c.id AND l.line_type = 'deposit')
     ORDER BY c.rn LIMIT 120
  LOOP
    v_refund_id := pg_temp.nanoid();
    v_at := v_order.created_at + make_interval(mins => floor(random() * 150 + 20)::int);
    v_method := CASE WHEN v_order.by_twint THEN 'twint' ELSE 'cash' END;

    SELECT id, quantity, unit_price_cents INTO v_dep
      FROM order_line
     WHERE order_id = v_order.id AND line_type = 'deposit'
     ORDER BY random() LIMIT 1;

    v_amount := v_dep.unit_price_cents * v_dep.quantity;

    INSERT INTO refund (id, tenant_id, order_id, amount_cents, kind, method, reason, actor_type, actor_id,
                        payrexx_transaction_id, status, created_at)
    VALUES (v_refund_id, v_tenant, v_order.id, v_amount, 'line', v_method::refund_method,
            'Depot-Rückgabe', 'operator', v_actor, v_order.payrexx_transaction_id, 'succeeded', v_at);

    INSERT INTO refund_line (id, tenant_id, refund_id, order_line_id, quantity, amount_cents)
    VALUES (pg_temp.nanoid(), v_tenant, v_refund_id, v_dep.id, v_dep.quantity, v_amount);

    UPDATE "order" SET status = 'partially_refunded', updated_at = v_at WHERE id = v_order.id;
  END LOOP;

  -- 5. Two orders cancelled by an admin without a payout: the stock comes back
  -- under its own reason, which is the other way cancellations reach the ledger.
  FOR v_order IN
    SELECT * FROM "order"
     WHERE tenant_id = v_tenant AND status = 'pending'
       AND created_at >= timestamptz '2026-08-14 00:00:00+02'
     ORDER BY random() LIMIT 2
  LOOP
    v_at := v_order.created_at + make_interval(mins => 40);
    INSERT INTO inventory_ledger (id, tenant_id, product_id, variant_id, delta, reason, order_id,
                                  order_line_id, created_at)
    SELECT pg_temp.nanoid(), v_tenant, l.product_id, l.variant_id, l.quantity, 'cancellation', v_order.id, l.id, v_at
      FROM order_line l
      JOIN product p ON p.id = l.product_id
     WHERE l.order_id = v_order.id
       AND l.line_type IN ('simple', 'component')
       AND p.type = 'simple';

    UPDATE "order" SET status = 'cancelled', updated_at = v_at WHERE id = v_order.id;
  END LOOP;
END
$seed$;

COMMIT;
