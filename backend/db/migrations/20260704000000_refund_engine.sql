-- atlas:txmode none
-- Refund engine: per-order refunds (line-level + arbitrary) and cancellations.
--
-- order_status gains 'partially_refunded'. Postgres forbids using a value added
-- by ALTER TYPE ... ADD VALUE in the same transaction it was added in, and Atlas
-- wraps a migration file in a transaction by default; the txmode none directive
-- above runs this file outside one so the enum extension is safe to replay.
-- Without that wrapping transaction a mid-file failure isn't rolled back, so the
-- type creations are guarded to stay replay-safe.

ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'partially_refunded';

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'refund_kind') THEN
    CREATE TYPE refund_kind AS ENUM ('line', 'arbitrary');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'refund_method') THEN
    CREATE TYPE refund_method AS ENUM ('cash', 'twint');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'refund_actor_type') THEN
    CREATE TYPE refund_actor_type AS ENUM ('operator', 'admin');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'refund_status') THEN
    CREATE TYPE refund_status AS ENUM ('pending', 'succeeded', 'failed');
  END IF;
END$$;

CREATE TABLE refund (
    id VARCHAR(12) PRIMARY KEY,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    order_id VARCHAR(12) NOT NULL REFERENCES "order" (id),
    amount_cents BIGINT NOT NULL,
    kind refund_kind NOT NULL,
    method refund_method NOT NULL,
    reason TEXT,
    actor_type refund_actor_type NOT NULL,
    actor_id TEXT NOT NULL,
    device_id VARCHAR(12),
    payrexx_transaction_id INTEGER,
    status refund_status NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX refund_tenant_id_idx ON refund (tenant_id);
CREATE INDEX refund_order_id_idx ON refund (order_id);

CREATE TABLE refund_line (
    id VARCHAR(12) PRIMARY KEY,
    tenant_id VARCHAR(12) NOT NULL REFERENCES tenant (id),
    refund_id VARCHAR(12) NOT NULL REFERENCES refund (id) ON DELETE CASCADE,
    order_line_id VARCHAR(12) NOT NULL REFERENCES order_line (id),
    quantity INTEGER NOT NULL,
    amount_cents BIGINT NOT NULL
);

CREATE INDEX refund_line_tenant_id_idx ON refund_line (tenant_id);
CREATE INDEX refund_line_refund_id_idx ON refund_line (refund_id);

ALTER TABLE refund ENABLE ROW LEVEL SECURITY;
CREATE POLICY refund_isolation ON refund FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

ALTER TABLE refund_line ENABLE ROW LEVEL SECURITY;
CREATE POLICY refund_line_isolation ON refund_line FOR ALL
  USING (tenant_id = app_current_tenant_id() OR app_is_platform_admin())
  WITH CHECK (tenant_id = app_current_tenant_id() OR app_is_platform_admin());

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON refund TO comanda_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON refund_line TO comanda_app;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'comanda_privileged') THEN
    GRANT ALL ON refund TO comanda_privileged;
    GRANT ALL ON refund_line TO comanda_privileged;
  END IF;
END$$;
