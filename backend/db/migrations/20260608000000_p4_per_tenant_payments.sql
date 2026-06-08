-- P4 (LEY-313): per-tenant payment + email provider credentials.
--
-- payment_webhook_token must be unique across tenants because it is resolved via
-- the privileged (RLS-bypassing) client before tenant context exists. The _enc
-- columns hold AES-256-GCM ciphertext; plaintext payment secrets are never
-- stored. payrexx_instance is plaintext (part of the API URL).
--
-- All columns are nullable/additive. Grants: added columns inherit the existing
-- settings table grant to comanda_app, so no new GRANT is required, and the RLS
-- isolation policy already covers them.
--
-- HAND-AUTHORED: `atlas migrate diff` is not runnable in the build sandbox.
-- Column types/sizes mirror internal/generated/ent/migrate/schema.go. A human
-- MUST verify this against a real `atlas migrate diff` before prod and run
-- `just migrate-hash` to refresh atlas.sum.

ALTER TABLE settings
  ADD COLUMN payment_webhook_token VARCHAR(64),
  ADD COLUMN payrexx_instance VARCHAR(100),
  ADD COLUMN payrexx_api_secret_enc TEXT,
  ADD COLUMN payrexx_webhook_secret_enc TEXT,
  ADD COLUMN payrexx_configured_at TIMESTAMPTZ,
  ADD COLUMN plunk_api_key_enc TEXT,
  ADD COLUMN plunk_from_name VARCHAR(100),
  ADD COLUMN plunk_from_email VARCHAR(255),
  ADD COLUMN plunk_reply_to VARCHAR(255),
  ADD COLUMN plunk_configured_at TIMESTAMPTZ;

-- Backfill: two dash-stripped gen_random_uuid() values give 64 opaque hex chars
-- (no pgcrypto needed), fitting VARCHAR(64). Runtime generation uses base64url;
-- both are opaque and unique, which is all the column requires.
UPDATE settings
  SET payment_webhook_token =
    replace(gen_random_uuid()::text, '-', '') || replace(gen_random_uuid()::text, '-', '')
  WHERE payment_webhook_token IS NULL;

CREATE UNIQUE INDEX settings_payment_webhook_token_key
  ON settings (payment_webhook_token);
