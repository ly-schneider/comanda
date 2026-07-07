CREATE TABLE station_enroll_token (
    id VARCHAR(12) PRIMARY KEY,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    token_enc BYTEA NOT NULL,
    issued_by VARCHAR(64) NOT NULL,
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One durable enrollment code per tenant.
CREATE UNIQUE INDEX station_enroll_token_tenant_id_key ON station_enroll_token (tenant_id);

ALTER TABLE station_enroll_token ENABLE ROW LEVEL SECURITY;
CREATE POLICY station_enroll_token_isolation ON station_enroll_token FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON station_enroll_token TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON station_enroll_token TO comanda_privileged;
  END IF;
END$$;
