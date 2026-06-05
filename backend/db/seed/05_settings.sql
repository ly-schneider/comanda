-- The settings singleton is keyed by the tenant UUID (see the tenant+RLS
-- migration, which renamed the legacy 'default' row to the bootstrap tenant id).
-- Seed runs as the privileged owner (bypasses RLS), so tenant_id is set
-- explicitly. Neutral branding values keep the dev seed un-branded.
INSERT INTO settings (
  id, tenant_id, pos_mode, club100_max_redemptions,
  display_name, currency,
  color_primary, color_primary_foreground, color_background, color_foreground
) VALUES (
  '00000000-0000-7000-8000-000000000001',
  '00000000-0000-7000-8000-000000000001',
  'JETON', 2,
  'Comanda', 'CHF',
  '#f87778', '#fdfdfd', '#e9e7e6', '#000000'
)
ON CONFLICT (id) DO UPDATE SET
  pos_mode = EXCLUDED.pos_mode,
  club100_max_redemptions = EXCLUDED.club100_max_redemptions,
  display_name = EXCLUDED.display_name,
  currency = EXCLUDED.currency;

INSERT INTO club100_free_product (settings_id, tenant_id, product_id) VALUES
  ('00000000-0000-7000-8000-000000000001', '00000000-0000-7000-8000-000000000001', '01970000-0000-7000-0002-000000000010'),  -- Smash Burger
  ('00000000-0000-7000-8000-000000000001', '00000000-0000-7000-8000-000000000001', '01970000-0000-7000-0002-000000000011')   -- Veggie Burger
ON CONFLICT (settings_id, product_id) DO NOTHING;
