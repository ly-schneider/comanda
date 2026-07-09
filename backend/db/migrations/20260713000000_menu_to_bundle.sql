ALTER TYPE product_type RENAME VALUE 'menu' TO 'bundle';

ALTER TABLE menu_slot RENAME TO bundle_slot;
ALTER TABLE menu_slot_option RENAME TO bundle_slot_option;

ALTER TABLE bundle_slot RENAME COLUMN menu_product_id TO bundle_product_id;
ALTER TABLE bundle_slot_option RENAME COLUMN menu_slot_id TO bundle_slot_id;

ALTER TABLE order_line RENAME COLUMN menu_slot_id TO bundle_slot_id;
ALTER TABLE order_line RENAME COLUMN menu_slot_name TO bundle_slot_name;

ALTER TABLE bundle_slot RENAME CONSTRAINT menu_slot_pkey TO bundle_slot_pkey;
ALTER TABLE bundle_slot RENAME CONSTRAINT menu_slot_menu_product_id_fkey TO bundle_slot_bundle_product_id_fkey;
ALTER TABLE bundle_slot RENAME CONSTRAINT menu_slot_tenant_id_fkey TO bundle_slot_tenant_id_fkey;

ALTER TABLE bundle_slot_option RENAME CONSTRAINT menu_slot_option_pkey TO bundle_slot_option_pkey;
ALTER TABLE bundle_slot_option RENAME CONSTRAINT menu_slot_option_menu_slot_id_fkey TO bundle_slot_option_bundle_slot_id_fkey;
ALTER TABLE bundle_slot_option RENAME CONSTRAINT menu_slot_option_option_product_id_fkey TO bundle_slot_option_option_product_id_fkey;
ALTER TABLE bundle_slot_option RENAME CONSTRAINT menu_slot_option_tenant_id_fkey TO bundle_slot_option_tenant_id_fkey;

ALTER TABLE order_line RENAME CONSTRAINT order_line_menu_slot_id_fkey TO order_line_bundle_slot_id_fkey;

ALTER INDEX idx_menu_slot_menu_product_id RENAME TO idx_bundle_slot_bundle_product_id;
ALTER INDEX idx_menu_slot_sequence RENAME TO idx_bundle_slot_sequence;
ALTER INDEX menu_slot_tenant_id_idx RENAME TO bundle_slot_tenant_id_idx;

ALTER INDEX idx_menu_slot_option_product_id RENAME TO idx_bundle_slot_option_product_id;
ALTER INDEX menu_slot_option_tenant_id_idx RENAME TO bundle_slot_option_tenant_id_idx;

ALTER INDEX idx_order_line_menu_slot_id RENAME TO idx_order_line_bundle_slot_id;

ALTER POLICY menu_slot_isolation ON bundle_slot RENAME TO bundle_slot_isolation;
ALTER POLICY menu_slot_option_isolation ON bundle_slot_option RENAME TO bundle_slot_option_isolation;
