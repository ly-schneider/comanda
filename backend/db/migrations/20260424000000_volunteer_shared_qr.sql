-- Refactor staff meals from per-slot orders to shared-QR with redemption counter.
-- Hard cutover: previous volunteer_campaign rows are truncated (feature unused in production).

TRUNCATE TABLE volunteer_campaign CASCADE;

DROP TABLE volunteer_slot;

ALTER TABLE volunteer_campaign
    ADD COLUMN max_redemptions  INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN redemption_count INTEGER NOT NULL DEFAULT 0,
    ADD CONSTRAINT volunteer_campaign_redeem_cap_ck CHECK (redemption_count <= max_redemptions),
    ADD CONSTRAINT volunteer_campaign_max_redemptions_positive_ck CHECK (max_redemptions > 0);

CREATE TABLE volunteer_redemption (
    id                VARCHAR(12) PRIMARY KEY,
    campaign_id       VARCHAR(12) NOT NULL REFERENCES volunteer_campaign (id) ON DELETE CASCADE,
    order_id          VARCHAR(12) NOT NULL UNIQUE REFERENCES "order" (id) ON DELETE RESTRICT,
    station_device_id VARCHAR(12) NULL REFERENCES device (id) ON DELETE SET NULL,
    idempotency_key   VARCHAR(64) NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_volunteer_redemption_campaign_id ON volunteer_redemption (campaign_id);
CREATE UNIQUE INDEX idx_volunteer_redemption_campaign_idempotency
    ON volunteer_redemption (campaign_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
