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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Version is the payload schema version emitted by Sign (fixed binary, v2).
// Bump only with a coordinated rollout across every verifier (web, Android).
const Version = 2

// Wire discriminator and fixed field widths for the v2 binary layout. The first
// body byte is the version/format discriminator (0x02); any other value is
// rejected as an unsupported version.
const (
	verV2 = 0x02

	idLen     = 12                        // tenant/order/product nanoids are 12 ASCII bytes
	headerLen = 1 + idLen + idLen + 4 + 1 // ver + tenant + order + issuedAt(u32) + count(u8) = 30
	lineLen   = idLen + 1                 // product id + qty(u8) = 13
	maxLines  = math.MaxUint8             // count is a uint8
	maxQty    = math.MaxUint8             // qty is a uint8
)

var (
	ErrMalformed          = errors.New("qrsign: malformed token")
	ErrBadSignature       = errors.New("qrsign: signature verification failed")
	ErrBadPayload         = errors.New("qrsign: invalid payload")
	ErrUnsupportedVersion = errors.New("qrsign: unsupported payload version")
)

type Line struct {
	ProductID string
	Quantity  int
}

// Payload is the decoded, verified pickup token, encoded/decoded positionally by
// encodeV2/decodeV2 (see doc.go).
type Payload struct {
	Version  int
	TenantID string
	OrderID  string
	IssuedAt int64
	Lines    []Line
}

// Sign returns base64url(no-pad) of the v2 binary body followed by its trailing
// 64-byte Ed25519 signature. The token stays base64url *text* on the wire (API
// responses, the Android bridge); only the QR writer decodes it to raw bytes.
func Sign(priv ed25519.PrivateKey, p Payload) (string, error) {
	body, err := encodeV2(p)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, body)
	token := make([]byte, 0, len(body)+len(sig))
	token = append(token, body...)
	token = append(token, sig...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

// encodeV2 lays out the fixed binary body. Every id must be exactly 12 bytes so
// the format stays fixed-width; counts and quantities must fit a uint8.
func encodeV2(p Payload) ([]byte, error) {
	if len(p.TenantID) != idLen {
		return nil, fmt.Errorf("%w: tenant id must be %d bytes, got %d", ErrBadPayload, idLen, len(p.TenantID))
	}
	if len(p.OrderID) != idLen {
		return nil, fmt.Errorf("%w: order id must be %d bytes, got %d", ErrBadPayload, idLen, len(p.OrderID))
	}
	if len(p.Lines) > maxLines {
		return nil, fmt.Errorf("%w: %d lines exceeds max %d", ErrBadPayload, len(p.Lines), maxLines)
	}
	if p.IssuedAt < 0 || p.IssuedAt > math.MaxUint32 {
		return nil, fmt.Errorf("%w: issued-at %d out of uint32 range", ErrBadPayload, p.IssuedAt)
	}

	body := make([]byte, 0, headerLen+len(p.Lines)*lineLen)
	body = append(body, verV2)
	body = append(body, p.TenantID...)
	body = append(body, p.OrderID...)
	body = binary.BigEndian.AppendUint32(body, uint32(p.IssuedAt))
	body = append(body, byte(len(p.Lines)))
	for _, l := range p.Lines {
		if len(l.ProductID) != idLen {
			return nil, fmt.Errorf("%w: product id must be %d bytes, got %d", ErrBadPayload, idLen, len(l.ProductID))
		}
		if l.Quantity < 1 || l.Quantity > maxQty {
			return nil, fmt.Errorf("%w: quantity %d out of range [1,%d]", ErrBadPayload, l.Quantity, maxQty)
		}
		body = append(body, l.ProductID...)
		body = append(body, byte(l.Quantity))
	}
	return body, nil
}

// Verify checks the signature over the literal decoded body bytes before
// parsing — never re-serializing, which would risk canonicalization drift
// between signer and verifier. Only the v2 binary format is accepted; any other
// leading byte is an unsupported version.
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

	if body[0] != verV2 {
		return Payload{}, ErrUnsupportedVersion
	}
	return decodeV2(body)
}

// decodeV2 parses a signature-verified v2 body. Any length/structure mismatch is
// a bad payload, not tampering (the signature already authenticated the bytes).
func decodeV2(body []byte) (Payload, error) {
	if len(body) < headerLen {
		return Payload{}, ErrBadPayload
	}
	count := int(body[29])
	if len(body) != headerLen+count*lineLen {
		return Payload{}, ErrBadPayload
	}
	p := Payload{
		Version:  Version,
		TenantID: string(body[1:13]),
		OrderID:  string(body[13:25]),
		IssuedAt: int64(binary.BigEndian.Uint32(body[25:29])),
		Lines:    make([]Line, count),
	}
	off := headerLen
	for i := 0; i < count; i++ {
		p.Lines[i] = Line{
			ProductID: string(body[off : off+idLen]),
			Quantity:  int(body[off+idLen]),
		}
		off += lineLen
	}
	return p, nil
}
