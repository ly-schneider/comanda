-- Replace the single durable enrollment token per tenant with N named presets,
-- each carrying its own token and an assigned product set. Clean drop + create:
-- the plugin has no production data, so no reshape/backfill.

DROP TABLE IF EXISTS station_enroll_token;

CREATE TABLE station_enroll_preset (
    id VARCHAR(12) PRIMARY KEY,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    name VARCHAR(60) NOT NULL,
    token_enc BYTEA NOT NULL,
    issued_by VARCHAR(64) NOT NULL,
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX station_enroll_preset_tenant_id_idx ON station_enroll_preset (tenant_id);

CREATE TABLE station_preset_product (
    preset_id VARCHAR(12) NOT NULL REFERENCES station_enroll_preset (id) ON DELETE CASCADE,
    product_id VARCHAR(12) NOT NULL REFERENCES product (id) ON DELETE CASCADE,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    PRIMARY KEY (preset_id, product_id)
);

CREATE INDEX station_preset_product_product_id_idx ON station_preset_product (product_id);

ALTER TABLE station_enroll_preset ENABLE ROW LEVEL SECURITY;
CREATE POLICY station_enroll_preset_isolation ON station_enroll_preset FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

ALTER TABLE station_preset_product ENABLE ROW LEVEL SECURITY;
CREATE POLICY station_preset_product_isolation ON station_preset_product FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON station_enroll_preset TO comanda_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON station_preset_product TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON station_enroll_preset TO comanda_privileged;
    GRANT ALL ON station_preset_product TO comanda_privileged;
  END IF;
END$$;
