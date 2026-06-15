-- Offline QR redemption: per-tenant Ed25519 signing keys and the signed QR token
-- persisted on the order.
--
-- settings.ed25519_private_seed_enc holds AES-256-GCM ciphertext (base64) of the
-- 32-byte Ed25519 private seed; the plaintext seed is never stored.
-- settings.ed25519_public_key holds base64url(no-pad) of the 32-byte public key,
-- stored in plaintext (public by design, served at GET /v1/qr-config).
-- "order".qr_payload holds the signed, self-contained QR token (see
-- internal/qrsign), persisted at creation so reprints don't re-sign.
--
-- The columns are additive on existing tenant-scoped RLS tables, inheriting each
-- table's comanda_app GRANT and tenant-isolation policy.

ALTER TABLE settings
  ADD COLUMN ed25519_private_seed_enc TEXT,
  ADD COLUMN ed25519_public_key TEXT;

ALTER TABLE "order"
  ADD COLUMN qr_payload TEXT;
