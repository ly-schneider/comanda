CREATE TYPE voucher_kind AS ENUM ('pass', 'entitlement');

ALTER TABLE voucher
    ADD COLUMN kind voucher_kind NOT NULL DEFAULT 'pass',
    ADD COLUMN products JSONB,
    ADD COLUMN batch_id VARCHAR(12),
    ADD COLUMN redeemed_at TIMESTAMPTZ,
    ADD COLUMN redeemed_order_id VARCHAR(12),
    ADD COLUMN redeemed_device_id VARCHAR(12);

CREATE INDEX voucher_tenant_id_batch_id_idx ON voucher (tenant_id, batch_id);
