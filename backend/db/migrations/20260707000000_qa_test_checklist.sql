-- Platform-admin manual-QA checklist: named runs with per-test results.
-- Platform-plane tables (no tenant_id); RLS gates on app_is_platform_admin().

CREATE TYPE qa_test_result_status AS ENUM ('success', 'error');

CREATE TABLE qa_test_run (
    id VARCHAR(12) PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE qa_test_result (
    id VARCHAR(12) PRIMARY KEY,
    run_id VARCHAR(12) NOT NULL REFERENCES qa_test_run (id) ON DELETE CASCADE,
    test_key VARCHAR(120) NOT NULL,
    status qa_test_result_status,
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX qa_test_result_run_id_test_key_idx ON qa_test_result (run_id, test_key);

ALTER TABLE qa_test_run ENABLE ROW LEVEL SECURITY;
CREATE POLICY qa_test_run_platform_only ON qa_test_run FOR ALL
  USING (app_is_platform_admin())
  WITH CHECK (app_is_platform_admin());

ALTER TABLE qa_test_result ENABLE ROW LEVEL SECURITY;
CREATE POLICY qa_test_result_platform_only ON qa_test_result FOR ALL
  USING (app_is_platform_admin())
  WITH CHECK (app_is_platform_admin());

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON qa_test_run TO comanda_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON qa_test_result TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON qa_test_run TO comanda_privileged;
    GRANT ALL ON qa_test_result TO comanda_privileged;
  END IF;
END$$;
