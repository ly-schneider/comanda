-- tenant_id has no DB default; default it to the bootstrap tenant for this seed.
ALTER TABLE category ALTER COLUMN tenant_id SET DEFAULT :'seed_tenant';

INSERT INTO category (id, name, is_active, position) VALUES
  ('cat________1', 'Getränke', true, 4),
  ('cat________2', 'Burgers', true, 2),
  ('cat________3', 'Beilagen', true, 3),
  ('cat________4', 'Menüs', true, 1)
ON CONFLICT (id) DO UPDATE SET tenant_id = :'seed_tenant';

ALTER TABLE category ALTER COLUMN tenant_id DROP DEFAULT;
