-- The settings singleton is keyed by the tenant id (see the tenant+RLS
-- migration, which renamed the legacy 'default' row to the bootstrap tenant id).
-- Seed runs as the privileged owner (bypasses RLS), so tenant_id is set
-- explicitly. Neutral branding values keep the dev seed un-branded.
INSERT INTO settings (
  id, tenant_id, pos_mode, club100_max_redemptions,
  display_name, currency,
  color_primary, color_primary_foreground, color_secondary,
  color_background, color_foreground, font_key, radius_key
) VALUES (
  'tenant_seed1',
  'tenant_seed1',
  'JETON', 2,
  'Comanda', 'CHF',
  '#0a0a0a', '#ffffff', '#f5f5f5',
  '#ffffff', '#0a0a0a', 'inter', 'rounded'
)
ON CONFLICT (id) DO UPDATE SET
  pos_mode = EXCLUDED.pos_mode,
  club100_max_redemptions = EXCLUDED.club100_max_redemptions,
  display_name = EXCLUDED.display_name,
  currency = EXCLUDED.currency,
  color_primary = EXCLUDED.color_primary,
  color_primary_foreground = EXCLUDED.color_primary_foreground,
  color_secondary = EXCLUDED.color_secondary,
  color_background = EXCLUDED.color_background,
  color_foreground = EXCLUDED.color_foreground,
  font_key = EXCLUDED.font_key,
  radius_key = EXCLUDED.radius_key;

INSERT INTO club100_free_product (settings_id, tenant_id, product_id) VALUES
  ('tenant_seed1', 'tenant_seed1', 'prd_______11'),  -- Smash Burger
  ('tenant_seed1', 'tenant_seed1', 'prd_______12')   -- Veggie Burger
ON CONFLICT (settings_id, product_id) DO NOTHING;
