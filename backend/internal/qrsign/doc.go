// Package qrsign — wire format contract
//
// This is the authoritative description of the signed QR token. The web
// verifier (@noble/ed25519) and the Android client MUST mirror it exactly.
// Byte-exact golden vectors live in qrsign_test.go and are reproduced in the
// web and Android test suites.
//
// # Algorithm
//
//	alg = Ed25519 (RFC 8032), 32-byte public key, 64-byte signature.
//
// # Payload (v3, fixed binary)
//
// The signed body is a fixed-width binary layout (network byte order). Every id
// is a 12-byte ASCII nanoid, so the record is fixed-width and needs no
// delimiters:
//
//	[0]        version = 0x03   (also the format discriminator)
//	[1..13)    tenant id, 12 ASCII bytes
//	[13..25)   order id, 12 ASCII bytes
//	[25..29)   issued-at, uint32 big-endian, Unix seconds (UTC)
//	[29]       line count, uint8
//	n ×        product id (12 ASCII bytes) + variant id (12 ASCII bytes) + qty (uint8, 1..255)
//	last 64    Ed25519 signature over all preceding bytes
//
// Lines are the redeemable physical items to hand over — simple products and
// the chosen bundle components. A configured bundle's parent is not listed;
// its components carry the items. Each line carries its variant id so the
// station can split same-product multi-variant lines; a product with no variant
// uses an all-zero (12 null bytes) variant id, which no nanoid can collide with.
//
// This is not encryption: the ids are readable ASCII inside the blob, exactly
// as they were inside the old decodable base64url body.
//
// # Token encoding
//
//	body  = the exact binary payload bytes (as signed)
//	sig   = ed25519.Sign(privateKey, body)            // 64 bytes
//	token = base64url_nopad( body || sig )            // sig is the LAST 64 bytes
//
// base64url is RFC 4648 §5 ("URL and filename safe") with NO padding. The token
// stays base64url *text* on the wire — API responses and the Android JS bridge
// carry it as text. Only the QR writer decodes it to raw bytes and emits a
// byte-mode QR segment (no base64url inside the QR); the QR reader hands those
// raw bytes straight to the verifier.
//
// # Format discrimination
//
// The first body byte is the version/format discriminator: after a successful
// signature check it MUST be 0x03 (v3 fixed binary, this document); any other
// value is rejected as an unsupported version. (The retired v1 format was
// canonical JSON, whose body started with '{' = 0x7B; v2 was the same fixed
// binary layout without the per-line variant id.)
//
// Discriminator allocation: the pickup-order token owns the low range — 0x03
// today, with 0x04..0x0F reserved for future pickup versions. The artist
// voucher token (see voucher.go) is a distinct format and starts a separate
// range at 0x10; the two verifiers mutually reject each other's discriminator.
//
// # Verification
//
//  1. base64url-decode the token (when it arrives as text) — or take the raw
//     bytes straight from the QR reader for a v3 QR.
//  2. The signature is the LAST 64 bytes; everything before it is the body.
//  3. ed25519.Verify(publicKey, body, sig) over the LITERAL body bytes. Never
//     re-serialize the decoded payload before verifying — re-serialization can
//     change byte order/spacing and break the signature.
//  4. Only after a successful verify, require the leading byte to be 0x03 and
//     parse the body as v3 binary.
//  5. REQUIRED version check: reject any body whose leading byte is not 0x03. A
//     valid signature does not imply a known schema.
//  6. RECOMMENDED freshness check: reject tokens whose issued-at is older than
//     the station's accepted window. On the fully-offline path there is no
//     server backstop against replay, so a freshness window bounds reuse.
//  7. REQUIRED multi-tenant check: compare the payload tenant id against the
//     station's own provisioned tenant id and REJECT on mismatch ("Wrong
//     event"). A valid signature only proves some tenant signed it; on the
//     fully-offline path there is no server backstop, so the station is the sole
//     enforcer of tenant scoping. Never skip this even when the signature
//     verifies.
//
// The public key is published unauthenticated at GET /v1/qr-config as
// base64url(no-pad) of the 32 raw public-key bytes. The private seed is stored
// AES-256-GCM-encrypted, never exposed.
package qrsign
