-- Adds BYO branding columns to the per-tenant settings singleton (P3 / LEY-318).
-- Additive only: nullable columns and columns with defaults, so existing rows
-- keep working (defaults backfill the non-nullable presentation fields).
--
-- HAND-AUTHORED: the `just migrate-diff` Atlas/Ent loader was not runnable in
-- the build sandbox (no Docker dev DB + no `to` source wired in atlas.hcl).
-- Column types mirror internal/generated/ent/migrate/schema.go exactly. A human
-- must verify this against a real `atlas migrate diff` before applying to prod.
ALTER TABLE settings
  ADD COLUMN display_name VARCHAR(100),
  ADD COLUMN logo_url TEXT,
  ADD COLUMN favicon_url TEXT,
  ADD COLUMN color_primary VARCHAR(9) NOT NULL DEFAULT '#f87778',
  ADD COLUMN color_primary_foreground VARCHAR(9) NOT NULL DEFAULT '#fdfdfd',
  ADD COLUMN color_background VARCHAR(9) NOT NULL DEFAULT '#e9e7e6',
  ADD COLUMN color_foreground VARCHAR(9) NOT NULL DEFAULT '#000000',
  ADD COLUMN contact_email VARCHAR(255),
  ADD COLUMN contact_phone VARCHAR(50),
  ADD COLUMN legal_name VARCHAR(200),
  ADD COLUMN legal_address TEXT,
  ADD COLUMN legal_text TEXT,
  ADD COLUMN currency VARCHAR(3) NOT NULL DEFAULT 'CHF',
  ADD COLUMN payment_provider VARCHAR(50) NOT NULL DEFAULT 'payrexx';
