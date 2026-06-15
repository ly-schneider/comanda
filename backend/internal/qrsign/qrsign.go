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
	ErrMalformed    = errors.New("qrsign: malformed token")
	ErrBadSignature = errors.New("qrsign: signature verification failed")
	ErrBadPayload   = errors.New("qrsign: invalid payload json")
)

type Line struct {
	ProductID string `json:"p"`
	Quantity  int    `json:"q"`
}

// Field order must never be reordered: the verifier checks the literal signed
// bytes and cross-language readers rely on the canonical key order v,t,o,i,l.
type Payload struct {
	Version  int    `json:"v"`
	TenantID string `json:"t"`
	OrderID  string `json:"o"`
	IssuedAt int64  `json:"i"`
	Lines    []Line `json:"l"`
}

// Sign returns base64url(no-pad) of the canonical payload JSON followed by its
// trailing 64-byte Ed25519 signature.
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

// Verify checks the signature over the literal decoded body bytes before
// unmarshaling — never re-serializing, which would risk JSON canonicalization
// drift between signer and verifier.
func Verify(pub ed25519.PublicKey, token string) (Payload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Payload{}, ErrMalformed
	}
	// <= not <: an exactly-64-byte token is an empty body and must be rejected.
	if len(raw) <= ed25519.SignatureSize {
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
