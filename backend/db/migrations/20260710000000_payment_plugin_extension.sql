-- Postgres cannot drop an enum value in place, so recreate payment_method.
-- Existing GRATIS_100CLUB rows migrate to gratis_plugin with plugin_key = 'club100'.
ALTER TABLE order_payment ADD COLUMN plugin_key VARCHAR NULL;
UPDATE order_payment SET plugin_key = 'club100' WHERE method::text = 'GRATIS_100CLUB';

ALTER TYPE payment_method RENAME TO payment_method_old;
CREATE TYPE payment_method AS ENUM ('CASH', 'CARD', 'TWINT', 'GRATIS_GUEST', 'GRATIS_VIP', 'GRATIS_STAFF', 'GRATIS_PLUGIN');
ALTER TABLE order_payment
  ALTER COLUMN method TYPE payment_method
  USING ((CASE WHEN method::text = 'GRATIS_100CLUB' THEN 'GRATIS_PLUGIN' ELSE method::text END)::payment_method);
DROP TYPE payment_method_old;

CREATE INDEX idx_order_payment_plugin_key ON order_payment (plugin_key) WHERE plugin_key IS NOT NULL;
