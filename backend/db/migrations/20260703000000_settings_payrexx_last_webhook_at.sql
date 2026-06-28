-- Records the last inbound Payrexx webhook whose HMAC signature verified — the
-- only honest proof the tenant's stored webhook secret matches what Payrexx
-- signs with. Surfaced in the Payrexx setup UI as a delivery confirmation.
--
-- Nullable/additive: the column inherits the existing settings table grant to
-- comanda_app and the RLS isolation policy, so no new GRANT/policy is required.
--
-- HAND-AUTHORED: `atlas migrate diff` is not runnable in the build sandbox.
-- The column type mirrors internal/generated/ent/migrate/schema.go. A human MUST
-- verify this against a real `atlas migrate diff` before prod and run
-- `just migrate-hash` to refresh atlas.sum.

ALTER TABLE settings
  ADD COLUMN payrexx_last_webhook_at TIMESTAMPTZ;
