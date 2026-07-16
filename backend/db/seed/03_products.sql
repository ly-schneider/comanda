-- tenant_id has no DB default; default it to the bootstrap tenant for this seed.
ALTER TABLE product ALTER COLUMN tenant_id SET DEFAULT :'seed_tenant';

INSERT INTO product (id, category_id, type, name, image, price_cents, jeton_id, is_active) VALUES
  ('prd________1', 'cat________1', 'simple', 'Coca Cola', '/assets/images/products/coca-cola.png', 350, 'tkn________3', true),
  ('prd________2', 'cat________1', 'simple', 'El Tony Mate', '/assets/images/products/el-tony-mate.png', 350, 'tkn________2', true),
  ('prd________3', 'cat________1', 'simple', 'Ice Tea Lemon', '/assets/images/products/ice-tea-lemon.png', 350, 'tkn________3', true),
  ('prd________4', 'cat________1', 'simple', 'Wasser', '/assets/images/products/wasser-prickelnd.png', 200, 'tkn________3', true),
  ('prd________6', 'cat________1', 'simple', 'Red Bull', '/assets/images/products/red-bull.png', 250, 'tkn________3', true),
  ('prd________7', 'cat________1', 'simple', 'Capri Sun', '/assets/images/products/capri-sun.png', 150, 'tkn________3', true)
ON CONFLICT (id) DO UPDATE SET tenant_id = :'seed_tenant';

INSERT INTO product (id, category_id, type, name, image, price_cents, jeton_id, is_active) VALUES
  ('prd_______11', 'cat________2', 'simple', 'Smash Burger', '/assets/images/products/smash-burger.png', 950, 'tkn________1', true),
  ('prd_______12', 'cat________2', 'simple', 'Veggie Burger', '/assets/images/products/veggie-burger.png', 1050, 'tkn________1', true)
ON CONFLICT (id) DO UPDATE SET tenant_id = :'seed_tenant';

INSERT INTO product (id, category_id, type, name, image, price_cents, jeton_id, is_active) VALUES
  ('prd_______21', 'cat________3', 'simple', 'Pommes', '/assets/images/products/pommes.png', 450, 'tkn________4', true)
ON CONFLICT (id) DO UPDATE SET tenant_id = :'seed_tenant';

ALTER TABLE product ALTER COLUMN tenant_id DROP DEFAULT;
