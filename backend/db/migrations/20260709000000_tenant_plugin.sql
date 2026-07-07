CREATE TABLE tenant_plugin (
    id VARCHAR(12) PRIMARY KEY,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    plugin_key VARCHAR(64) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    config JSONB,
    secrets_enc BYTEA,
    secrets_configured_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX tenant_plugin_tenant_id_idx ON tenant_plugin (tenant_id);
CREATE UNIQUE INDEX tenant_plugin_tenant_key_unique ON tenant_plugin (tenant_id, plugin_key);

ALTER TABLE tenant_plugin ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_plugin_isolation ON tenant_plugin FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_plugin TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON tenant_plugin TO comanda_privileged;
  END IF;
END$$;
