-- tenant_id has no DB default; default it to the bootstrap tenant for this seed.
ALTER TABLE product ALTER COLUMN tenant_id SET DEFAULT 'tenant_seed1';
ALTER TABLE menu_slot ALTER COLUMN tenant_id SET DEFAULT 'tenant_seed1';
ALTER TABLE menu_slot_option ALTER COLUMN tenant_id SET DEFAULT 'tenant_seed1';

-- Menu products (in Menüs category)
INSERT INTO product (id, category_id, type, name, image, price_cents, jeton_id, is_active) VALUES
  ('prd_menu___1', 'cat________4', 'menu', 'Menü Klein', '/assets/images/products/menu-1-klein.png', 1200, NULL, true),
  ('prd_menu___2', 'cat________4', 'menu', 'Menü Gross', '/assets/images/products/menu-2-gross.png', 1500, NULL, true)
ON CONFLICT (id) DO NOTHING;

-- Menü Klein (Burger + Getränk)
INSERT INTO menu_slot (id, menu_product_id, name, sequence) VALUES
  ('mns________1', 'prd_menu___1', 'Burger', 1),
  ('mns________2', 'prd_menu___1', 'Getränk', 2)
ON CONFLICT (id) DO NOTHING;

-- Menü Gross (Burger + Beilage + Getränk)
INSERT INTO menu_slot (id, menu_product_id, name, sequence) VALUES
  ('mns_______11', 'prd_menu___2', 'Burger', 1),
  ('mns_______12', 'prd_menu___2', 'Beilage', 2),
  ('mns_______13', 'prd_menu___2', 'Getränk', 3)
ON CONFLICT (id) DO NOTHING;

-- Menü Klein
INSERT INTO menu_slot_option (menu_slot_id, option_product_id) VALUES
  -- Burgers
  ('mns________1', 'prd_______11'),
  ('mns________1', 'prd_______12'),
  -- Getränke
  ('mns________2', 'prd________1'),
  ('mns________2', 'prd________2'),
  ('mns________2', 'prd________3'),
  ('mns________2', 'prd________4'),
  ('mns________2', 'prd________6')
ON CONFLICT DO NOTHING;

-- Menü Gross
INSERT INTO menu_slot_option (menu_slot_id, option_product_id) VALUES
  -- Burgers
  ('mns_______11', 'prd_______11'),
  ('mns_______11', 'prd_______12'),
  -- Pommes
  ('mns_______12', 'prd_______21'),
  -- Getränke
  ('mns_______13', 'prd________1'),
  ('mns_______13', 'prd________2'),
  ('mns_______13', 'prd________3'),
  ('mns_______13', 'prd________4'),
  ('mns_______13', 'prd________6')
ON CONFLICT DO NOTHING;

ALTER TABLE product ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE menu_slot ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE menu_slot_option ALTER COLUMN tenant_id DROP DEFAULT;
