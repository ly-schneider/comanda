-- squawk-ignore-file renaming-table, renaming-column
-- The 100 Club plugin becomes a membership plugin that asks an external
-- provider. The provider owns the member list and the budget, so the columns
-- that named Elvanto become provider-agnostic ones, and a release the provider
-- confirmed is remembered here.
ALTER TABLE club100_redemption RENAME TO membership_redemption;

ALTER TABLE membership_redemption RENAME COLUMN elvanto_person_id TO member_ref;
ALTER TABLE membership_redemption RENAME COLUMN elvanto_person_name TO member_name;
ALTER TABLE membership_redemption RENAME COLUMN free_product_quantity TO quantity;
ALTER TABLE membership_redemption ALTER COLUMN member_ref TYPE VARCHAR(100);
ALTER TABLE membership_redemption ALTER COLUMN member_name DROP NOT NULL;
ALTER TABLE membership_redemption ADD COLUMN released_at TIMESTAMPTZ;

ALTER TABLE membership_redemption RENAME CONSTRAINT club100_redemption_pkey TO membership_redemption_pkey;
ALTER TABLE membership_redemption RENAME CONSTRAINT club100_redemption_order_id_fkey TO membership_redemption_order_id_fkey;
ALTER TABLE membership_redemption RENAME CONSTRAINT club100_redemption_tenant_id_fkey TO membership_redemption_tenant_id_fkey;

ALTER INDEX club100_redemption_tenant_id_idx RENAME TO membership_redemption_tenant_id_idx;
ALTER POLICY club100_redemption_isolation ON membership_redemption RENAME TO membership_redemption_isolation;

-- A refund asks what an order booked; nothing looks a member up by hand.
DROP INDEX idx_club100_elvanto_person;
CREATE INDEX membership_redemption_order_id_idx ON membership_redemption (order_id);

-- Enablement and the free-product list carry over. The per-person cap does not:
-- the provider counts now, and two caps would fight. The stored Elvanto
-- credentials mean nothing to a provider, so the tenant is asked for new ones.
UPDATE tenant_plugin
   SET plugin_key = 'membership',
       config = config - 'maxRedemptions',
       secrets_enc = NULL,
       secrets_configured_at = NULL
 WHERE plugin_key = 'club100';

UPDATE order_payment SET plugin_key = 'membership' WHERE plugin_key = 'club100';
