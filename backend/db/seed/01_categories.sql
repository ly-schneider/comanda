-- tenant_id is NOT NULL with no DB default; the seed runs as the table owner
-- (bypasses RLS), so set a transient default to the bootstrap tenant, then drop it.
ALTER TABLE category ALTER COLUMN tenant_id SET DEFAULT '00000000-0000-7000-8000-000000000001';

INSERT INTO category (id, name, is_active, position) VALUES
  ('01970000-0000-7000-0000-000000000001', 'Getränke', true, 4),
  ('01970000-0000-7000-0000-000000000002', 'Burgers', true, 2),
  ('01970000-0000-7000-0000-000000000003', 'Beilagen', true, 3),
  ('01970000-0000-7000-0000-000000000004', 'Menüs', true, 1)
ON CONFLICT (id) DO NOTHING;

ALTER TABLE category ALTER COLUMN tenant_id DROP DEFAULT;
