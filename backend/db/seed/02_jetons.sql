-- tenant_id has no DB default; default it to the bootstrap tenant for this seed.
ALTER TABLE jeton ALTER COLUMN tenant_id SET DEFAULT :'seed_tenant';

INSERT INTO jeton (id, name, color) VALUES
  ('tkn________1', 'Burger', '#EF4444'),
  ('tkn________2', 'El Tony Maté', '#3B82F6'),
  ('tkn________3', 'Getränk', '#22C55E'),
  ('tkn________4', 'Beilage', '#EAB308')
ON CONFLICT (id) DO UPDATE SET tenant_id = :'seed_tenant';

ALTER TABLE jeton ALTER COLUMN tenant_id DROP DEFAULT;
