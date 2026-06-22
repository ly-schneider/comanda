-- Domain ownership is proven by publishing this random token as a TXT record at
-- _comanda-verify.<hostname>. Nullable: pre-existing rows (seeded hosts) have none.
ALTER TABLE tenant_domain ADD COLUMN verification_token TEXT;
