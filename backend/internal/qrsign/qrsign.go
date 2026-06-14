// Package qrsign produces and verifies self-contained, Ed25519-signed QR
// payloads that a pickup station can verify fully offline.
//
// See doc.go for the exact wire format. The format is a cross-language contract:
// the web verifier (@noble/ed25519) and the Android client mirror it byte for
// byte, so changes here are breaking changes for every reader.
package qrsign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// Version is the current payload schema version. Bump only with a coordinated
// rollout across every verifier (web, Android).
const Version = 1

var (
	// ErrMalformed is returned when the token is not valid base64url or is too
	// short to contain a 64-byte signature.
	ErrMalformed = errors.New("qrsign: malformed token")
	// ErrBadSignature is returned when the signature does not verify against the
	// given public key (tampering, wrong key, or truncation).
	ErrBadSignature = errors.New("qrsign: signature verification failed")
	// ErrBadPayload is returned when the signed prefix is not valid payload JSON.
	ErrBadPayload = errors.New("qrsign: invalid payload json")
)

// Line is one redeemable product line: a product id and the redeemed quantity.
type Line struct {
	ProductID string `json:"p"`
	Quantity  int    `json:"q"`
}

// Payload is the signed body. Field order is fixed by the struct definition and
// must never be reordered — the verifier checks the literal signed bytes, and
// downstream JSON consumers in other languages rely on the canonical key order
// "v","t","o","i","l".
type Payload struct {
	Version  int    `json:"v"`
	TenantID string `json:"t"`
	OrderID  string `json:"o"`
	IssuedAt int64  `json:"i"`
	Lines    []Line `json:"l"`
}

// Sign canonically marshals p, signs the exact JSON bytes with priv, and returns
// base64url(no-pad) of payloadJSON||sig (sig is the trailing 64 bytes).
func Sign(priv ed25519.PrivateKey, p Payload) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, body)
	token := make([]byte, 0, len(body)+len(sig))
	token = append(token, body...)
	token = append(token, sig...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

// Verify decodes token, splits the trailing 64 bytes as the signature, verifies
// the literal prefix bytes against pub (never re-serializing), and only then
// unmarshals the prefix into a Payload. Verifying the literal bytes avoids any
// JSON canonicalization drift between signer and verifier.
func Verify(pub ed25519.PublicKey, token string) (Payload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Payload{}, ErrMalformed
	}
	if len(raw) < ed25519.SignatureSize {
		return Payload{}, ErrMalformed
	}
	split := len(raw) - ed25519.SignatureSize
	body, sig := raw[:split], raw[split:]

	if len(pub) != ed25519.PublicKeySize {
		return Payload{}, ErrBadSignature
	}
	if !ed25519.Verify(pub, body, sig) {
		return Payload{}, ErrBadSignature
	}

	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		return Payload{}, ErrBadPayload
	}
	return p, nil
}
