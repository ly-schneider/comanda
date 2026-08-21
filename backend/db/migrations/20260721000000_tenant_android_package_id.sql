ALTER TABLE tenant ADD COLUMN android_package_id VARCHAR(66);

-- Slugs are DNS labels ([a-z0-9-], never underscore), so hyphen->underscore is
-- injective and cannot collide two tenants onto one package. The leading "t_"
-- keeps the segment starting with a letter for slugs that begin with a digit.
UPDATE tenant SET android_package_id = 't_' || replace(slug, '-', '_');

CREATE UNIQUE INDEX tenant_android_package_id_key ON tenant (android_package_id);
