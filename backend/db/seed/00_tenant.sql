-- Creates the tenant :tenant_slug with its Better Auth organization, a settings
-- singleton, and :tenant_host as its verified primary domain — the same shape
-- platform-admin provisioning produces, for local use where that UI needs a
-- platform admin that doesn't exist yet.
--
-- Idempotent: an existing tenant keeps its id and only gains what it is missing.

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

\o /dev/null
SELECT set_config('seed.tenant_slug', lower(:'tenant_slug'), false),
       set_config('seed.tenant_name', :'tenant_name', false),
       set_config('seed.tenant_host', lower(:'tenant_host'), false);
\o

DO $$
DECLARE
  v_slug      text := current_setting('seed.tenant_slug');
  v_name      text := current_setting('seed.tenant_name');
  v_host      text := current_setting('seed.tenant_host');
  v_tenant_id text;
  v_org_id    text;
  v_host_owner text;
BEGIN
  SELECT id, organization_id INTO v_tenant_id, v_org_id FROM tenant WHERE slug = v_slug;

  IF v_tenant_id IS NULL THEN
    v_tenant_id := pg_temp.nanoid();
    v_org_id := pg_temp.nanoid();
    INSERT INTO "organization" (id, name, slug, created_at)
      VALUES (v_org_id, v_name, v_slug, now());
    INSERT INTO tenant (id, slug, name, organization_id, android_package_id)
      VALUES (v_tenant_id, v_slug, v_name, v_org_id, 't_' || replace(v_slug, '-', '_'));
    RAISE NOTICE 'created tenant % (%)', v_slug, v_tenant_id;
  ELSE
    IF v_org_id IS NULL THEN
      v_org_id := pg_temp.nanoid();
      INSERT INTO "organization" (id, name, slug, created_at)
        VALUES (v_org_id, v_name, v_slug, now());
      UPDATE tenant SET organization_id = v_org_id WHERE id = v_tenant_id;
    END IF;
    -- Self-heal tenants seeded before the column existed; the Ent field is
    -- immutable, so this is the only path that can fill it.
    UPDATE tenant SET android_package_id = 't_' || replace(v_slug, '-', '_')
      WHERE id = v_tenant_id AND android_package_id IS NULL;
    RAISE NOTICE 'tenant % already exists (%)', v_slug, v_tenant_id;
  END IF;

  INSERT INTO settings (id, tenant_id) VALUES (v_tenant_id, v_tenant_id)
    ON CONFLICT (id) DO NOTHING;

  SELECT tenant_id INTO v_host_owner FROM tenant_domain WHERE hostname = v_host;

  IF v_host_owner IS NULL THEN
    -- Platform-assigned: verified by construction and token-less, which marks it
    -- as undeletable by a tenant admin.
    INSERT INTO tenant_domain (id, tenant_id, hostname, is_primary, verified_at)
      VALUES (pg_temp.nanoid(), v_tenant_id, v_host, NOT EXISTS (
        SELECT 1 FROM tenant_domain WHERE tenant_id = v_tenant_id AND is_primary
      ), now());
    RAISE NOTICE 'domain % → %', v_host, v_slug;
  ELSIF v_host_owner <> v_tenant_id THEN
    RAISE EXCEPTION 'host % already belongs to another tenant (%)', v_host, v_host_owner;
  END IF;
END $$;
