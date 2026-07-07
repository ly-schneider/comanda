-- Postgres cannot drop an enum value in place, so recreate payment_method.
ALTER TYPE payment_method RENAME TO payment_method_old;
CREATE TYPE payment_method AS ENUM ('CASH', 'CARD', 'TWINT', 'GRATIS_GUEST', 'GRATIS_VIP', 'GRATIS_STAFF', 'GRATIS_PLUGIN');
ALTER TABLE order_payment
  ALTER COLUMN method TYPE payment_method USING (method::text::payment_method);
DROP TYPE payment_method_old;

ALTER TABLE order_payment ADD COLUMN plugin_key VARCHAR NULL;

-- Analytics filter: "paid via <plugin>" is `plugin_key = '<key>'` (with or
-- without method = gratis_plugin, for partial redemptions on paid methods).
CREATE INDEX idx_order_payment_plugin_key ON order_payment (plugin_key) WHERE plugin_key IS NOT NULL;
