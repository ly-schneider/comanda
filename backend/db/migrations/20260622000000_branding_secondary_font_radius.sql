-- Extends per-tenant branding with a secondary accent color, a curated font
-- key, and a corner-style (radius) key, and shifts the neutral base palette to
-- the fixed white/monochrome design (tenants override the accents on top).
--
-- Additive + default changes only. Existing rows keep their stored colors; the
-- new defaults apply to rows created from here on. font_key/radius_key backfill
-- to the curated defaults for existing rows.
--
-- HAND-AUTHORED (mirrors 20260605000000_p3_branding_settings.sql): the
-- `just migrate-diff` Atlas/Ent loader is not runnable in the build sandbox.
-- Column types/defaults mirror internal/generated/ent/migrate/schema.go exactly.
-- A human must verify this against a real `atlas migrate diff` before prod.
ALTER TABLE settings
  ADD COLUMN color_secondary VARCHAR(9) NOT NULL DEFAULT '#f5f5f5',
  ADD COLUMN font_key VARCHAR(32) NOT NULL DEFAULT 'inter',
  ADD COLUMN radius_key VARCHAR(16) NOT NULL DEFAULT 'rounded';

ALTER TABLE settings
  ALTER COLUMN color_primary SET DEFAULT '#0a0a0a',
  ALTER COLUMN color_primary_foreground SET DEFAULT '#ffffff',
  ALTER COLUMN color_background SET DEFAULT '#ffffff',
  ALTER COLUMN color_foreground SET DEFAULT '#0a0a0a';
