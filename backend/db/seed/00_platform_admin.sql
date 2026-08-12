-- Makes :admin_email a platform admin and the owner of the tenant :seed_slug.
-- Sign-in is OTP/OAuth only, so a user row with a verified email is everything
-- Better Auth needs — no account row, no password. Sign in at the tenant host
-- and read the OTP from Mailpit (http://localhost:8025).
--
-- Idempotent: re-running promotes an existing user instead of creating one.

CREATE OR REPLACE FUNCTION pg_temp.nanoid() RETURNS varchar(12)
LANGUAGE sql VOLATILE AS $$
  SELECT string_agg(substr('123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz',
                           (floor(random()*53))::int + 1, 1), '')
  FROM generate_series(1, 12)
$$;

-- psql does not interpolate :vars inside dollar-quoted bodies, so they are
-- handed to the block as session GUCs.
\o /dev/null
SELECT set_config('seed.admin_email', lower(:'admin_email'), false),
       set_config('seed.admin_name', :'admin_name', false),
       set_config('seed.tenant_slug', :'seed_slug', false);
\o

DO $$
DECLARE
  v_email   text := current_setting('seed.admin_email');
  v_name    text := current_setting('seed.admin_name');
  v_slug    text := current_setting('seed.tenant_slug');
  v_user_id text;
  v_org_id  text;
  v_tenant_id text;
BEGIN
  SELECT id, organization_id INTO v_tenant_id, v_org_id FROM tenant WHERE slug = v_slug;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'no tenant with slug %', v_slug;
  END IF;

  IF v_org_id IS NULL THEN
    RAISE EXCEPTION 'tenant % has no organization; membership cannot be granted', v_slug;
  END IF;

  SELECT id INTO v_user_id FROM "user" WHERE lower(email) = v_email;

  IF v_user_id IS NULL THEN
    v_user_id := pg_temp.nanoid();
    INSERT INTO "user" (id, name, email, email_verified, is_platform_admin)
      VALUES (v_user_id, v_name, v_email, true, true);
    RAISE NOTICE 'created user % (%)', v_email, v_user_id;
  ELSE
    UPDATE "user" SET is_platform_admin = true, email_verified = true WHERE id = v_user_id;
    RAISE NOTICE 'promoted existing user % (%)', v_email, v_user_id;
  END IF;

  INSERT INTO "member" (id, organization_id, user_id, role, created_at)
    VALUES (pg_temp.nanoid(), v_org_id, v_user_id, 'owner', now())
    ON CONFLICT (organization_id, user_id) DO UPDATE SET role = 'owner';

  -- Migrations create the bootstrap tenant but not its settings singleton, which
  -- branding reads on every request.
  INSERT INTO settings (id, tenant_id) VALUES (v_tenant_id, v_tenant_id)
    ON CONFLICT (id) DO NOTHING;

  RAISE NOTICE 'owner of tenant % (org %) and platform admin', v_slug, v_org_id;
END $$;
