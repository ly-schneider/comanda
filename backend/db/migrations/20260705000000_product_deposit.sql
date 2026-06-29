-- atlas:txmode none
-- Per-product returnable-container deposit: a per-unit, refundable charge that
-- rides on its own order line.
--
-- order_item_type gains 'deposit'. Postgres forbids using a value added by
-- ALTER TYPE ... ADD VALUE in the same transaction it was added in, and Atlas
-- wraps a migration file in a transaction by default; the txmode none directive
-- runs this file outside one so the enum extension is safe to replay. Without
-- that wrapping transaction a mid-file failure isn't rolled back, so the column
-- changes below are guarded to stay replay-safe.

ALTER TYPE order_item_type ADD VALUE IF NOT EXISTS 'deposit';

ALTER TABLE product ADD COLUMN IF NOT EXISTS deposit_cents BIGINT NOT NULL DEFAULT 0;

-- Deposit lines carry the title "Depot — {product}", which can exceed the old
-- 20-char cap; widening a varchar is a metadata-only change (no table rewrite).
ALTER TABLE order_line ALTER COLUMN title TYPE VARCHAR(40);
