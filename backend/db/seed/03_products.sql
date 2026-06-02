INSERT INTO product (id, category_id, type, name, image, price_cents, jeton_id, is_active) VALUES
  ('01970000-0000-7000-0002-000000000001', '01970000-0000-7000-0000-000000000001', 'simple', 'Coca Cola', '/assets/images/products/coca-cola.png', 350, '01970000-0000-7000-0001-000000000003', true),
  ('01970000-0000-7000-0002-000000000002', '01970000-0000-7000-0000-000000000001', 'simple', 'El Tony Mate', '/assets/images/products/el-tony-mate.png', 350, '01970000-0000-7000-0001-000000000002', true),
  ('01970000-0000-7000-0002-000000000003', '01970000-0000-7000-0000-000000000001', 'simple', 'Ice Tea Lemon', '/assets/images/products/ice-tea-lemon.png', 350, '01970000-0000-7000-0001-000000000003', true),
  ('01970000-0000-7000-0002-000000000004', '01970000-0000-7000-0000-000000000001', 'simple', 'Wasser', '/assets/images/products/wasser-prickelnd.png', 200, '01970000-0000-7000-0001-000000000003', true),
  ('01970000-0000-7000-0002-000000000006', '01970000-0000-7000-0000-000000000001', 'simple', 'Red Bull', '/assets/images/products/red-bull.png', 250, '01970000-0000-7000-0001-000000000003', true),
  ('01970000-0000-7000-0002-000000000007', '01970000-0000-7000-0000-000000000001', 'simple', 'Capri Sun', '/assets/images/products/capri-sun.png', 150, '01970000-0000-7000-0001-000000000003', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO product (id, category_id, type, name, image, price_cents, jeton_id, is_active) VALUES
  ('01970000-0000-7000-0002-000000000010', '01970000-0000-7000-0000-000000000002', 'simple', 'Smash Burger', '/assets/images/products/smash-burger.png', 950, '01970000-0000-7000-0001-000000000001', true),
  ('01970000-0000-7000-0002-000000000011', '01970000-0000-7000-0000-000000000002', 'simple', 'Veggie Burger', '/assets/images/products/veggie-burger.png', 1050, '01970000-0000-7000-0001-000000000001', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO product (id, category_id, type, name, image, price_cents, jeton_id, is_active) VALUES
  ('01970000-0000-7000-0002-000000000020', '01970000-0000-7000-0000-000000000003', 'simple', 'Pommes', '/assets/images/products/pommes.png', 450, '01970000-0000-7000-0001-000000000004', true)
ON CONFLICT (id) DO NOTHING;
