// Package qrsign produces and verifies self-contained, Ed25519-signed QR
// payloads that a pickup station can verify fully offline.
//
// See doc.go for the exact wire format. The format is a cross-language contract:
// the web verifier (@noble/ed25519) and the Android client mirror it byte for
// byte, so changes here are breaking changes for every reader.
package qrsign

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Bump only with a coordinated rollout across the web + Android verifiers.
const Version = 3

const (
	verV3 = 0x03

	idLen     = 12                        // tenant/order/product/variant nanoids are 12 ASCII bytes
	headerLen = 1 + idLen + idLen + 4 + 1 // ver + 2 ids + issuedAt(u32) + count(u8) = 30
	lineLen   = idLen + idLen + 1         // product id + variant id + qty(u8) = 25
	maxLines  = math.MaxUint8
	maxQty    = math.MaxUint8
)

// noVariant is the 12-byte sentinel for a line with no variant. Nanoids never
// contain null bytes, so an all-zero variant id is unambiguously "none".
var noVariant = make([]byte, idLen)

var (
	ErrMalformed          = errors.New("qrsign: malformed token")
	ErrBadSignature       = errors.New("qrsign: signature verification failed")
	ErrBadPayload         = errors.New("qrsign: invalid payload")
	ErrUnsupportedVersion = errors.New("qrsign: unsupported payload version")
)

type Line struct {
	ProductID string
	// VariantID is the 12-byte variant nanoid, or "" for a product with no variant.
	VariantID string
	Quantity  int
}

type Payload struct {
	Version  int
	TenantID string
	OrderID  string
	IssuedAt int64
	Lines    []Line
}

// The token stays base64url *text* on the wire (API responses, Android bridge);
// only the QR writer decodes it to the raw bytes that go into the QR.
func Sign(priv ed25519.PrivateKey, p Payload) (string, error) {
	body, err := encodeV3(p)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, body)
	token := make([]byte, 0, len(body)+len(sig))
	token = append(token, body...)
	token = append(token, sig...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

// Ids must be exactly 12 bytes so the record stays fixed-width and delimiter-free.
func encodeV3(p Payload) ([]byte, error) {
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
	body = append(body, verV3)
	body = append(body, p.TenantID...)
	body = append(body, p.OrderID...)
	body = binary.BigEndian.AppendUint32(body, uint32(p.IssuedAt))
	body = append(body, byte(len(p.Lines)))
	for _, l := range p.Lines {
		if len(l.ProductID) != idLen {
			return nil, fmt.Errorf("%w: product id must be %d bytes, got %d", ErrBadPayload, idLen, len(l.ProductID))
		}
		if l.VariantID != "" && len(l.VariantID) != idLen {
			return nil, fmt.Errorf("%w: variant id must be %d bytes or empty, got %d", ErrBadPayload, idLen, len(l.VariantID))
		}
		if l.Quantity < 1 || l.Quantity > maxQty {
			return nil, fmt.Errorf("%w: quantity %d out of range [1,%d]", ErrBadPayload, l.Quantity, maxQty)
		}
		body = append(body, l.ProductID...)
		if l.VariantID == "" {
			body = append(body, noVariant...)
		} else {
			body = append(body, l.VariantID...)
		}
		body = append(body, byte(l.Quantity))
	}
	return body, nil
}

// Verify checks the signature over the literal decoded bytes — never
// re-serializing, which would risk canonicalization drift between signer and reader.
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

	if body[0] != verV3 {
		return Payload{}, ErrUnsupportedVersion
	}
	return decodeV3(body)
}

// A length/structure mismatch here is a bad payload, not tampering — the
// signature already authenticated these bytes.
func decodeV3(body []byte) (Payload, error) {
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
		variant := ""
		if raw := body[off+idLen : off+2*idLen]; !bytes.Equal(raw, noVariant) {
			variant = string(raw)
		}
		p.Lines[i] = Line{
			ProductID: string(body[off : off+idLen]),
			VariantID: variant,
			Quantity:  int(body[off+2*idLen]),
		}
		off += lineLen
	}
	return p, nil
}
