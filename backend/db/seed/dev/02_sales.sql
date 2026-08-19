-- Dev-only: generates a three-day event's worth of sales for a tenant, shaped
-- the way the app writes them — bundle parents priced with their components at
-- zero, deposits as sibling lines, one payment row per settled order, a sale
-- ledger row per physical item. Sized and mixed after a real BlessThun weekend.
--
-- Usage: psql "$DATABASE_URL" -v seed_tenant=<tenant id> -f 02_sales.sql
-- Undo:  99_reset.sql (keys off the same day window)
--
-- Runs as one transaction: a failure leaves the database untouched rather than
-- half an event.

\set ON_ERROR_STOP on

BEGIN;

-- Event days are wall-clock Swiss days, not UTC ones.
SET LOCAL TIME ZONE 'Europe/Zurich';
SELECT set_config('seed.tenant', :'seed_tenant', true);

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

-- Weighted pick: the smallest -ln(U)/w wins with probability proportional to w.
CREATE OR REPLACE FUNCTION pg_temp.pick_hour() RETURNS int
LANGUAGE sql VOLATILE AS $$
  SELECT h FROM (VALUES
    (11,2),(12,7),(13,6),(14,3),(15,3),(16,4),(17,8),(18,13),(19,17),(20,15),(21,12),(22,8),(23,4)
  ) AS t(h, w) ORDER BY -ln(random())/w LIMIT 1
$$;

CREATE TEMP TABLE seed_pool (product_id varchar(12), weight int) ON COMMIT DROP;
INSERT INTO seed_pool VALUES
  ('prd________1', 26),  -- Coca Cola
  ('prd________3', 20),  -- Ice Tea Lemon
  ('prd________2', 12),  -- El Tony Mate
  ('prd________4', 14),  -- Wasser
  ('prd________6', 9),   -- Red Bull
  ('prd________7', 7),   -- Capri Sun
  ('prd_______11', 18),  -- Smash Burger
  ('prd_______12', 7),   -- Veggie Burger
  ('prd_______21', 15),  -- Pommes
  ('prd_bndl___1', 13),  -- Menü Klein
  ('prd_bndl___2', 10),  -- Menü Gross
  ('prd_dev_shrt', 3),   -- BlessThun Shirt
  ('prd_dev_3f2_', 1);   -- Shirt 3 für 2

CREATE TEMP TABLE seed_pos_device (id varchar(12)) ON COMMIT DROP;

DO $seed$
DECLARE
  v_tenant      text := current_setting('seed.tenant');
  v_days        date[] := ARRAY['2026-08-14', '2026-08-15', '2026-08-16']::date[];
  v_counts      int[]  := ARRAY[800, 1150, 600];
  v_day         date;
  v_day_idx     int;
  v_order_no    int;
  v_created     timestamptz;
  v_order_id    varchar(12);
  v_origin      text;
  v_method      text;
  v_status      text;
  v_total       bigint;
  v_items       int;
  v_item        int;
  v_qty         int;
  v_prod_id     varchar(12);
  v_prod_name   text;
  v_prod_type   text;
  v_prod_price  bigint;
  v_prod_dep    bigint;
  v_var_id      varchar(12);
  v_var_value   varchar(20);
  v_var_adj     bigint;
  v_slot        record;
  v_opt_id      varchar(12);
  v_opt_name    text;
  v_parent_id   varchar(12);
  v_line_id     varchar(12);
  v_unit        bigint;
  v_roll        numeric;
  v_device      varchar(12);
BEGIN
  -- Two tills, so POS payments carry a device the way they do in the hall.
  FOR v_item IN 1..2 LOOP
    v_line_id := pg_temp.nanoid();
    INSERT INTO device (id, tenant_id, name, device_key, type, status, decided_by, decided_at, created_at)
    VALUES (v_line_id, v_tenant, 'Kasse ' || v_item, 'dev-seed-kasse-' || v_item, 'POS', 'approved',
            'kBHQDv91PRkx', timestamptz '2026-08-14 09:00:00+02', timestamptz '2026-08-14 09:00:00+02');
    INSERT INTO seed_pos_device VALUES (v_line_id);
  END LOOP;

  FOR v_day_idx IN 1..array_length(v_days, 1) LOOP
    v_day := v_days[v_day_idx];

    FOR v_order_no IN 1..v_counts[v_day_idx] LOOP
      v_created := v_day
        + make_interval(hours => pg_temp.pick_hour())
        + make_interval(mins => floor(random() * 60)::int, secs => floor(random() * 60)::int);

      v_roll := random();
      IF v_roll < 0.78 THEN
        v_origin := 'shop';
        v_method := 'TWINT';
      ELSE
        v_origin := 'pos';
        v_roll := random();
        v_method := CASE
          WHEN v_roll < 0.50 THEN 'CASH'
          WHEN v_roll < 0.92 THEN 'CARD'
          WHEN v_roll < 0.96 THEN 'GRATIS_GUEST'
          WHEN v_roll < 0.99 THEN 'GRATIS_VIP'
          ELSE 'GRATIS_STAFF'
        END;
      END IF;

      -- A few checkouts are simply abandoned. They reserved stock but never took
      -- money, so the revenue report has to leave them out.
      v_status := CASE WHEN random() < 0.012 THEN 'pending' ELSE 'paid' END;

      v_order_id := pg_temp.nanoid();
      v_total := 0;

      INSERT INTO "order" (id, tenant_id, total_cents, status, origin, created_at, updated_at,
                           contact_email, payment_attempt_id, payrexx_gateway_id, payrexx_transaction_id)
      VALUES (
        v_order_id, v_tenant, 0, v_status::order_status, v_origin::order_origin, v_created, v_created,
        CASE WHEN v_origin = 'shop' AND random() < 0.85
             THEN 'gast' || floor(random() * 9000 + 1000)::text || '@example.ch' END,
        CASE WHEN v_origin = 'shop' THEN 'att_' || pg_temp.nanoid() END,
        CASE WHEN v_origin = 'shop' AND v_status = 'paid' THEN floor(random() * 900000 + 100000)::int END,
        CASE WHEN v_origin = 'shop' AND v_status = 'paid' THEN floor(random() * 900000 + 100000)::int END
      );

      v_roll := random();
      v_items := CASE WHEN v_roll < 0.55 THEN 1 WHEN v_roll < 0.85 THEN 2 ELSE 3 END;

      FOR v_item IN 1..v_items LOOP
        SELECT p.id, p.name, p.type::text, p.price_cents, p.deposit_cents
          INTO v_prod_id, v_prod_name, v_prod_type, v_prod_price, v_prod_dep
          FROM seed_pool sp
          JOIN product p ON p.id = sp.product_id AND p.tenant_id = v_tenant
         ORDER BY -ln(random()) / sp.weight
         LIMIT 1;

        v_roll := random();
        v_qty := CASE WHEN v_roll < 0.76 THEN 1 WHEN v_roll < 0.95 THEN 2 ELSE 3 END;
        v_parent_id := pg_temp.nanoid();

        IF v_prod_type = 'bundle' THEN
          INSERT INTO order_line (id, tenant_id, order_id, line_type, product_id, title, quantity, unit_price_cents)
          VALUES (v_parent_id, v_tenant, v_order_id, 'bundle', v_prod_id, left(v_prod_name, 40), v_qty, v_prod_price);
          v_total := v_total + v_prod_price * v_qty;

          FOR v_slot IN
            SELECT id, name FROM bundle_slot
             WHERE bundle_product_id = v_prod_id AND tenant_id = v_tenant
             ORDER BY sequence
          LOOP
            SELECT p.id, p.name INTO v_opt_id, v_opt_name
              FROM bundle_slot_option bso
              JOIN product p ON p.id = bso.option_product_id
             WHERE bso.bundle_slot_id = v_slot.id
             ORDER BY random() LIMIT 1;
            CONTINUE WHEN v_opt_id IS NULL;

            INSERT INTO order_line (id, tenant_id, order_id, line_type, product_id, title, quantity,
                                    unit_price_cents, parent_line_id, bundle_slot_id, bundle_slot_name)
            VALUES (pg_temp.nanoid(), v_tenant, v_order_id, 'component', v_opt_id, left(v_opt_name, 40), v_qty,
                    0, v_parent_id, v_slot.id, left(v_slot.name, 20));

            -- Checkout does not stamp order_line_id on a sale row; only the
            -- restore paths do. Mirror that or the export would look better here
            -- than it does in production.
            INSERT INTO inventory_ledger (id, tenant_id, product_id, delta, reason, order_id, created_at)
            VALUES (pg_temp.nanoid(), v_tenant, v_opt_id, -v_qty, 'sale', v_order_id, v_created);
          END LOOP;
        ELSE
          SELECT id, value, price_adjustment_cents
            INTO v_var_id, v_var_value, v_var_adj
            FROM product_variant
           WHERE product_id = v_prod_id AND tenant_id = v_tenant AND is_active
           ORDER BY random() LIMIT 1;

          v_unit := v_prod_price + COALESCE(v_var_adj, 0);

          INSERT INTO order_line (id, tenant_id, order_id, line_type, product_id, title, quantity,
                                  unit_price_cents, variant_id, variant_value)
          VALUES (v_parent_id, v_tenant, v_order_id, 'simple', v_prod_id, left(v_prod_name, 40), v_qty,
                  v_unit, v_var_id, v_var_value);
          v_total := v_total + v_unit * v_qty;

          IF v_prod_dep > 0 THEN
            INSERT INTO order_line (id, tenant_id, order_id, line_type, product_id, title, quantity,
                                    unit_price_cents, parent_line_id)
            VALUES (pg_temp.nanoid(), v_tenant, v_order_id, 'deposit', v_prod_id,
                    left('Depot — ' || v_prod_name, 40), v_qty, v_prod_dep, v_parent_id);
            v_total := v_total + v_prod_dep * v_qty;
          END IF;

          INSERT INTO inventory_ledger (id, tenant_id, product_id, variant_id, delta, reason, order_id, created_at)
          VALUES (pg_temp.nanoid(), v_tenant, v_prod_id, v_var_id, -v_qty, 'sale', v_order_id, v_created);
        END IF;
      END LOOP;

      UPDATE "order" SET total_cents = v_total WHERE id = v_order_id;

      IF v_status = 'paid' THEN
        v_device := NULL;
        IF v_origin = 'pos' THEN
          SELECT id INTO v_device FROM seed_pos_device ORDER BY random() LIMIT 1;
        END IF;

        INSERT INTO order_payment (id, tenant_id, order_id, method, amount_cents, device_id, paid_at,
                                   card_brand, card_last4, entry_mode, card_transaction_id)
        VALUES (
          pg_temp.nanoid(), v_tenant, v_order_id, v_method::payment_method, v_total, v_device,
          v_created + make_interval(secs => floor(random() * 90 + 10)::int),
          CASE WHEN v_method = 'CARD' THEN (ARRAY['VISA','MASTERCARD','MAESTRO','AMEX'])[floor(random()*4)+1] END,
          CASE WHEN v_method = 'CARD' THEN lpad(floor(random()*10000)::text, 4, '0') END,
          CASE WHEN v_method = 'CARD' THEN (ARRAY['CONTACTLESS','CONTACTLESS','CHIP'])[floor(random()*3)+1] END,
          CASE WHEN v_method = 'CARD' THEN upper(substr(md5(random()::text), 1, 8)) END
        );
      END IF;
    END LOOP;
  END LOOP;
END
$seed$;

COMMIT;
