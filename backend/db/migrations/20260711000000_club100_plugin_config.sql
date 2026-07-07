-- Club100 config moves to tenant_plugin.config; no data migration (no prod data).
ALTER TABLE settings DROP COLUMN club100_max_redemptions;

DROP TABLE club100_free_product;
