-- tenant_id is NOT NULL with no DB default; the seed runs as the table owner
-- (bypasses RLS), so set a transient default to the bootstrap tenant, then drop it.
ALTER TABLE jeton ALTER COLUMN tenant_id SET DEFAULT '00000000-0000-7000-8000-000000000001';

INSERT INTO jeton (id, name, color) VALUES
  ('01970000-0000-7000-0001-000000000001', 'Burger', '#EF4444'),
  ('01970000-0000-7000-0001-000000000002', 'El Tony Maté', '#3B82F6'),
  ('01970000-0000-7000-0001-000000000003', 'Getränk', '#22C55E'),
  ('01970000-0000-7000-0001-000000000004', 'Beilage', '#EAB308')
ON CONFLICT (id) DO NOTHING;

ALTER TABLE jeton ALTER COLUMN tenant_id DROP DEFAULT;
