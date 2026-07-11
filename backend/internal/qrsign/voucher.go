package qrsign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
)

const verV3 = 0x03

const (
	// disc + tenant id + voucher id + validFrom(u32) + validUntil(u32) = 33.
	voucherHeaderLen = 1 + idLen + idLen + 4 + 4
	maxLabelLen      = math.MaxUint8
	maxCategories    = math.MaxUint8
	maxCatNameLen    = math.MaxUint8
)

// VoucherPayload is the artist-voucher display token. Categories are the
// human-readable NAMES snapshotted at issue time; category ids never leave the
// database, so a scanned voucher reveals only what it displays.
type VoucherPayload struct {
	TenantID   string
	VoucherID  string
	ValidFrom  int64
	ValidUntil int64
	Label      string
	Categories []string
}

// SignVoucher signs a voucher payload with the tenant's Ed25519 key, producing a
// base64url-nopad token whose first body byte is the 0x03 discriminator.
func SignVoucher(priv ed25519.PrivateKey, p VoucherPayload) (string, error) {
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

func encodeV3(p VoucherPayload) ([]byte, error) {
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
	if len(p.Categories) > maxCategories {
		return nil, fmt.Errorf("%w: %d categories exceeds max %d", ErrBadPayload, len(p.Categories), maxCategories)
	}

	body := make([]byte, 0, voucherHeaderLen+1+len(p.Label)+1)
	body = append(body, verV3)
	body = append(body, p.TenantID...)
	body = append(body, p.VoucherID...)
	body = binary.BigEndian.AppendUint32(body, uint32(p.ValidFrom))
	body = binary.BigEndian.AppendUint32(body, uint32(p.ValidUntil))
	body = append(body, byte(len(p.Label)))
	body = append(body, p.Label...)
	body = append(body, byte(len(p.Categories)))
	for _, name := range p.Categories {
		if len(name) == 0 || len(name) > maxCatNameLen {
			return nil, fmt.Errorf("%w: category name length %d out of range [1,%d]", ErrBadPayload, len(name), maxCatNameLen)
		}
		body = append(body, byte(len(name)))
		body = append(body, name...)
	}
	return body, nil
}

// VerifyVoucher checks the signature over the literal decoded bytes and parses a
// voucher payload. It rejects any token whose discriminator is not 0x03, so a
// pickup (0x02) token verified here fails as an unsupported version.
func VerifyVoucher(pub ed25519.PublicKey, token string) (VoucherPayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return VoucherPayload{}, ErrMalformed
	}
	if len(raw) <= ed25519.SignatureSize {
		return VoucherPayload{}, ErrMalformed
	}
	split := len(raw) - ed25519.SignatureSize
	body, sig := raw[:split], raw[split:]

	if len(pub) != ed25519.PublicKeySize {
		return VoucherPayload{}, ErrBadSignature
	}
	if !ed25519.Verify(pub, body, sig) {
		return VoucherPayload{}, ErrBadSignature
	}

	if body[0] != verV3 {
		return VoucherPayload{}, ErrUnsupportedVersion
	}
	return decodeV3(body)
}

func decodeV3(body []byte) (VoucherPayload, error) {
	if len(body) < voucherHeaderLen+1 {
		return VoucherPayload{}, ErrBadPayload
	}
	labelLen := int(body[voucherHeaderLen])
	off := voucherHeaderLen + 1
	if len(body) < off+labelLen+1 {
		return VoucherPayload{}, ErrBadPayload
	}
	label := string(body[off : off+labelLen])
	off += labelLen

	catCount := int(body[off])
	off++
	cats := make([]string, 0, catCount)
	for i := 0; i < catCount; i++ {
		if len(body) < off+1 {
			return VoucherPayload{}, ErrBadPayload
		}
		n := int(body[off])
		off++
		if len(body) < off+n {
			return VoucherPayload{}, ErrBadPayload
		}
		cats = append(cats, string(body[off:off+n]))
		off += n
	}
	if off != len(body) {
		return VoucherPayload{}, ErrBadPayload
	}

	return VoucherPayload{
		TenantID:   string(body[1 : 1+idLen]),
		VoucherID:  string(body[1+idLen : 1+idLen+idLen]),
		ValidFrom:  int64(binary.BigEndian.Uint32(body[voucherHeaderLen-8 : voucherHeaderLen-4])),
		ValidUntil: int64(binary.BigEndian.Uint32(body[voucherHeaderLen-4 : voucherHeaderLen])),
		Label:      label,
		Categories: cats,
	}, nil
}
