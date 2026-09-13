package qrsign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
)

const verEntitlement = 0x11

const (
	entLineLen  = idLen + 1 // product id + qty(u8)
	maxEntLines = math.MaxUint8
)

type EntitlementLine struct {
	ProductID string
	Quantity  int
}

type EntitlementPayload struct {
	TenantID   string
	VoucherID  string
	ValidFrom  int64
	ValidUntil int64
	Label      string
	Lines      []EntitlementLine
}

func SignEntitlement(priv ed25519.PrivateKey, p EntitlementPayload) (string, error) {
	body, err := encodeEntitlement(p)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, body)
	token := make([]byte, 0, len(body)+len(sig))
	token = append(token, body...)
	token = append(token, sig...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func encodeEntitlement(p EntitlementPayload) ([]byte, error) {
	if len(p.TenantID) != idLen {
		return nil, fmt.Errorf("%w: tenant id must be %d bytes, got %d", ErrBadPayload, idLen, len(p.TenantID))
	}
	if len(p.VoucherID) != idLen {
		return nil, fmt.Errorf("%w: voucher id must be %d bytes, got %d", ErrBadPayload, idLen, len(p.VoucherID))
	}
	if p.ValidFrom < 0 || p.ValidFrom > math.MaxUint32 {
		return nil, fmt.Errorf("%w: valid-from %d out of uint32 range", ErrBadPayload, p.ValidFrom)
	}
	if p.ValidUntil < 0 || p.ValidUntil > math.MaxUint32 {
		return nil, fmt.Errorf("%w: valid-until %d out of uint32 range", ErrBadPayload, p.ValidUntil)
	}
	if len(p.Label) == 0 || len(p.Label) > maxLabelLen {
		return nil, fmt.Errorf("%w: label length %d out of range [1,%d]", ErrBadPayload, len(p.Label), maxLabelLen)
	}
	if len(p.Lines) == 0 || len(p.Lines) > maxEntLines {
		return nil, fmt.Errorf("%w: line count %d out of range [1,%d]", ErrBadPayload, len(p.Lines), maxEntLines)
	}

	body := make([]byte, 0, voucherHeaderLen+1+len(p.Label)+1+len(p.Lines)*entLineLen)
	body = append(body, verEntitlement)
	body = append(body, p.TenantID...)
	body = append(body, p.VoucherID...)
	body = binary.BigEndian.AppendUint32(body, uint32(p.ValidFrom))
	body = binary.BigEndian.AppendUint32(body, uint32(p.ValidUntil))
	body = append(body, byte(len(p.Label)))
	body = append(body, p.Label...)
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

func VerifyEntitlement(pub ed25519.PublicKey, token string) (EntitlementPayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return EntitlementPayload{}, ErrMalformed
	}
	if len(raw) <= ed25519.SignatureSize {
		return EntitlementPayload{}, ErrMalformed
	}
	split := len(raw) - ed25519.SignatureSize
	body, sig := raw[:split], raw[split:]

	if len(pub) != ed25519.PublicKeySize {
		return EntitlementPayload{}, ErrBadSignature
	}
	if !ed25519.Verify(pub, body, sig) {
		return EntitlementPayload{}, ErrBadSignature
	}

	if body[0] != verEntitlement {
		return EntitlementPayload{}, ErrUnsupportedVersion
	}
	return decodeEntitlement(body)
}

func decodeEntitlement(body []byte) (EntitlementPayload, error) {
	if len(body) < voucherHeaderLen+1 {
		return EntitlementPayload{}, ErrBadPayload
	}
	labelLen := int(body[voucherHeaderLen])
	off := voucherHeaderLen + 1
	if len(body) < off+labelLen+1 {
		return EntitlementPayload{}, ErrBadPayload
	}
	label := string(body[off : off+labelLen])
	off += labelLen

	count := int(body[off])
	off++
	if count == 0 || len(body) != off+count*entLineLen {
		return EntitlementPayload{}, ErrBadPayload
	}
	lines := make([]EntitlementLine, count)
	for i := 0; i < count; i++ {
		qty := int(body[off+idLen])
		if qty == 0 {
			return EntitlementPayload{}, ErrBadPayload
		}
		lines[i] = EntitlementLine{
			ProductID: string(body[off : off+idLen]),
			Quantity:  qty,
		}
		off += entLineLen
	}

	return EntitlementPayload{
		TenantID:   string(body[1 : 1+idLen]),
		VoucherID:  string(body[1+idLen : 1+idLen+idLen]),
		ValidFrom:  int64(binary.BigEndian.Uint32(body[voucherHeaderLen-8 : voucherHeaderLen-4])),
		ValidUntil: int64(binary.BigEndian.Uint32(body[voucherHeaderLen-4 : voucherHeaderLen])),
		Label:      label,
		Lines:      lines,
	}, nil
}
