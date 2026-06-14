-- LEY-324 (P1): offline QR redemption — per-tenant Ed25519 signing keys + the
-- signed QR token persisted on the order.
--
-- settings.ed25519_private_seed_enc holds AES-256-GCM ciphertext (base64) of the
-- 32-byte Ed25519 private seed; the plaintext seed is never stored.
-- settings.ed25519_public_key holds base64url(no-pad) of the 32-byte public key,
-- stored in PLAINTEXT (it is public by design and served at GET /v1/qr-config).
-- "order".qr_payload holds the signed, self-contained QR token (see
-- internal/qrsign), persisted at creation so reprints don't re-sign.
--
-- All columns are nullable/additive. settings and "order" are existing
-- tenant-scoped RLS tables; the added columns inherit each table's existing
-- comanda_app GRANT and its tenant-isolation RLS policy, so no new GRANT or
-- policy is required.
--
-- HAND-AUTHORED: `atlas migrate diff` is not runnable in the build sandbox.
-- Column types mirror internal/generated/ent/migrate/schema.go (TypeString,
-- Size 2147483647 -> TEXT). A human MUST verify against a real `atlas migrate
-- diff` before prod and run `just migrate-hash` to refresh atlas.sum.

ALTER TABLE settings
  ADD COLUMN ed25519_private_seed_enc TEXT,
  ADD COLUMN ed25519_public_key TEXT;

ALTER TABLE "order"
  ADD COLUMN qr_payload TEXT;
