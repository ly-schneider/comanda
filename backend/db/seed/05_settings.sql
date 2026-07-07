-- The settings singleton is keyed by the tenant id (see the tenant+RLS
-- migration, which renamed the legacy 'default' row to the bootstrap tenant id).
-- Seed runs as the privileged owner (bypasses RLS), so tenant_id is set
-- explicitly. Neutral branding values keep the dev seed un-branded.
INSERT INTO settings (
  id, tenant_id, pos_mode,
  display_name, currency,
  color_primary, color_primary_foreground, color_secondary,
  color_background, color_foreground, font_key, radius_key
) VALUES (
  'tenant_seed1',
  'tenant_seed1',
  'JETON',
  'Comanda', 'CHF',
  '#0a0a0a', '#ffffff', '#f5f5f5',
  '#ffffff', '#0a0a0a', 'inter', 'rounded'
)
ON CONFLICT (id) DO UPDATE SET
  pos_mode = EXCLUDED.pos_mode,
  display_name = EXCLUDED.display_name,
  currency = EXCLUDED.currency,
  color_primary = EXCLUDED.color_primary,
  color_primary_foreground = EXCLUDED.color_primary_foreground,
  color_secondary = EXCLUDED.color_secondary,
  color_background = EXCLUDED.color_background,
  color_foreground = EXCLUDED.color_foreground,
  font_key = EXCLUDED.font_key,
  radius_key = EXCLUDED.radius_key;

-- club100 enabled for the dev tenant; Elvanto secrets set via platform-admin.
INSERT INTO tenant_plugin (id, tenant_id, plugin_key, enabled, config) VALUES (
  'tplg_seed100', 'tenant_seed1', 'club100', true,
  '{"maxRedemptions":2,"freeProductIds":["prd_______11","prd_______12"]}'
)
ON CONFLICT (tenant_id, plugin_key) DO UPDATE SET
  enabled = EXCLUDED.enabled,
  config = EXCLUDED.config;

-- stationqr enabled for the dev tenant; no config or secrets required.
INSERT INTO tenant_plugin (id, tenant_id, plugin_key, enabled) VALUES (
  'tplg_seedsqr', 'tenant_seed1', 'stationqr', true
)
ON CONFLICT (tenant_id, plugin_key) DO UPDATE SET
  enabled = EXCLUDED.enabled;
