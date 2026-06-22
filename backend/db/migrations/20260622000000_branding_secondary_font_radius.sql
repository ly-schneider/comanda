-- Existing rows keep their stored colors; the new column/default values apply to
-- rows created from here on.
ALTER TABLE settings
  ADD COLUMN color_secondary VARCHAR(9) NOT NULL DEFAULT '#f5f5f5',
  ADD COLUMN font_key VARCHAR(32) NOT NULL DEFAULT 'inter',
  ADD COLUMN radius_key VARCHAR(16) NOT NULL DEFAULT 'rounded';

ALTER TABLE settings
  ALTER COLUMN color_primary SET DEFAULT '#0a0a0a',
  ALTER COLUMN color_primary_foreground SET DEFAULT '#ffffff',
  ALTER COLUMN color_background SET DEFAULT '#ffffff',
  ALTER COLUMN color_foreground SET DEFAULT '#0a0a0a';
