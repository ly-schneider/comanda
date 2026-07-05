-- Product variants: selectable options on simple products (e.g. t-shirt sizes),
-- each with a signed price adjustment on top of the product's base price and
-- its own availability. Order lines and inventory ledger rows carry the chosen
-- variant; variant_value on order_line is a display snapshot that survives
-- variant archival or deletion.
--
-- Pre-variant inventory_ledger rows (variant_id IS NULL) stay as product-level
-- history; per-variant stock sums start at zero and are seeded via
-- opening_balance entries carrying a variant_id.

CREATE TABLE product_variant (
    id VARCHAR(12) PRIMARY KEY,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    product_id VARCHAR(12) NOT NULL REFERENCES product (id),
    value VARCHAR(20) NOT NULL,
    price_adjustment_cents BIGINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    sequence INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX product_variant_tenant_id_idx ON product_variant (tenant_id);
CREATE INDEX product_variant_product_id_idx ON product_variant (product_id);
CREATE UNIQUE INDEX product_variant_tenant_id_product_id_value_idx
    ON product_variant (tenant_id, product_id, value);

ALTER TABLE product ADD COLUMN variant_label VARCHAR(20);

ALTER TABLE order_line ADD COLUMN variant_id VARCHAR(12) REFERENCES product_variant (id);
ALTER TABLE order_line ADD COLUMN variant_value VARCHAR(20);
CREATE INDEX order_line_variant_id_idx ON order_line (variant_id);

ALTER TABLE inventory_ledger ADD COLUMN variant_id VARCHAR(12) REFERENCES product_variant (id);
CREATE INDEX inventory_ledger_variant_id_idx ON inventory_ledger (variant_id);

ALTER TABLE product_variant ENABLE ROW LEVEL SECURITY;
CREATE POLICY product_variant_isolation ON product_variant FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON product_variant TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON product_variant TO comanda_privileged;
  END IF;
END$$;
