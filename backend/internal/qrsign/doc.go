// Package qrsign — wire format contract
//
// This is the authoritative description of the signed QR token. The web P4
// verifier (@noble/ed25519) and the Android client MUST mirror it exactly.
//
// # Algorithm
//
//	alg = Ed25519 (RFC 8032), 32-byte public key, 64-byte signature.
//
// # Payload
//
// The signed body is canonical JSON with a FIXED key order. It is produced by
// marshalling the Payload struct (never a map), so the bytes are deterministic:
//
//	{"v":1,"t":"<tenantId>","o":"<orderId>","i":<issuedAtUnixSeconds>,"l":[{"p":"<productId>","q":<qty>}, ...]}
//
//	v  payload schema version, currently 1
//	t  tenant id
//	o  order id
//	i  issued-at, Unix seconds (UTC)
//	l  redeemable items: one {p,q} entry per physical item to hand over —
//	     simple products and the chosen menu components. A configured menu's
//	     bundle parent is not listed; its components carry the items.
//	     p = product id, q = quantity (>= 1)
//
// # Token encoding
//
//	body  = the exact payload JSON bytes (as signed)
//	sig   = ed25519.Sign(privateKey, body)            // 64 bytes
//	token = base64url_nopad( body || sig )            // sig is the LAST 64 bytes
//
// base64url is RFC 4648 §5 ("URL and filename safe") with NO padding.
//
// # Verification
//
//  1. base64url-decode the token.
//  2. The signature is the LAST 64 bytes; everything before it is the body.
//  3. ed25519.Verify(publicKey, body, sig) over the LITERAL body bytes.
//     Never re-serialize the decoded payload before verifying — re-serialization
//     can change byte order/spacing and break the signature.
//  4. Only after a successful verify, JSON-unmarshal body into the payload.
//  5. REQUIRED version check: reject any payload whose `v` is not a version this
//     reader understands. A valid signature does not imply a known schema.
//  6. RECOMMENDED freshness check: reject tokens whose `i` (issued-at) is older
//     than the station's accepted window. On the fully-offline path there is no
//     server backstop against replay, so a freshness window bounds reuse.
//  7. REQUIRED multi-tenant check: compare payload.t against the station's own
//     provisioned tenant id and REJECT on mismatch ("Wrong event"). A valid
//     signature only proves some tenant signed it; on the fully-offline path
//     there is no server backstop, so the station is the sole enforcer of
//     tenant scoping. Never skip this even when the signature verifies.
//
// The public key is published unauthenticated at GET /v1/qr-config as
// base64url(no-pad) of the 32 raw public-key bytes. The private seed is stored
// AES-256-GCM-encrypted, never exposed.
package qrsign
