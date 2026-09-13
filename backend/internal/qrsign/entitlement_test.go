package qrsign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"reflect"
	"testing"
)

// Canonical cross-language golden vector for the 0x11 entitlement token. The
// PDF generator and the web/Android readers reproduce these exact bytes; any
// change here is a coordinated wire-format change for every reader.
const (
	goldenEntitlementBodyHex = "1174656e616e745f7365656431456e743741623243643945666553f100655542800e537472617373656e616b74696f6e0370726f645f706f6d6d657331010070726f645f62757267657231010170726f645f636865657365310101"
	goldenEntitlementToken   = "EXRlbmFudF9zZWVkMUVudDdBYjJDZDlFZmVT8QBlVUKADlN0cmFzc2VuYWt0aW9uA3Byb2RfcG9tbWVzMQEAcHJvZF9idXJnZXIxAQFwcm9kX2NoZWVzZTEBAdPsxohp6QXgYcD6rGBQ6Kx6EcFufqcWSgdetV4pzr-mLD_ZBu7yXd_9uWl8ZTJsVGY_99TamduToPpXOjFAfgI"
)

func goldenEntitlement() EntitlementPayload {
	return EntitlementPayload{
		TenantID:   "tenant_seed1",
		VoucherID:  "Ent7Ab2Cd9Ef",
		ValidFrom:  1_700_000_000,
		ValidUntil: 1_700_086_400,
		Label:      "Strassenaktion",
		Lines: []EntitlementLine{
			{ProductID: "prod_pommes1", Quantity: 1, ChoiceGroup: 0},
			{ProductID: "prod_burger1", Quantity: 1, ChoiceGroup: 1},
			{ProductID: "prod_cheese1", Quantity: 1, ChoiceGroup: 1},
		},
	}
}

func sampleEntitlement() EntitlementPayload {
	return EntitlementPayload{
		TenantID:   "tenant_abc12",
		VoucherID:  "voucher_xyz9",
		ValidFrom:  1_700_000_000,
		ValidUntil: 1_700_100_000,
		Label:      "Outreach",
		Lines: []EntitlementLine{
			{ProductID: "prod_aaa1111", Quantity: 1},
		},
	}
}

func TestEntitlementGoldenVector(t *testing.T) {
	pub, priv := goldenKey(t)

	body, err := encodeEntitlement(goldenEntitlement())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := hex.EncodeToString(body); got != goldenEntitlementBodyHex {
		t.Fatalf("body drift:\n got %s\nwant %s", got, goldenEntitlementBodyHex)
	}

	token, err := SignEntitlement(priv, goldenEntitlement())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if token != goldenEntitlementToken {
		t.Fatalf("token drift:\n got %s\nwant %s", token, goldenEntitlementToken)
	}

	got, err := VerifyEntitlement(pub, goldenEntitlementToken)
	if err != nil {
		t.Fatalf("verify golden: %v", err)
	}
	if !reflect.DeepEqual(got, goldenEntitlement()) {
		t.Fatalf("golden payload mismatch:\n got %+v\nwant %+v", got, goldenEntitlement())
	}
}

func TestEntitlementRoundTrip(t *testing.T) {
	pub, priv := mustKey(t)
	p := sampleEntitlement()
	p.Lines = append(p.Lines, EntitlementLine{ProductID: "prod_bbb2222", Quantity: 255})

	token, err := SignEntitlement(priv, p)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := VerifyEntitlement(pub, token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Fatalf("payload mismatch:\n got %+v\nwant %+v", got, p)
	}
}

func TestEntitlementDeterministicEncoding(t *testing.T) {
	_, priv := mustKey(t)
	p := sampleEntitlement()
	a, err := SignEntitlement(priv, p)
	if err != nil {
		t.Fatalf("sign a: %v", err)
	}
	b, err := SignEntitlement(priv, p)
	if err != nil {
		t.Fatalf("sign b: %v", err)
	}
	if a != b {
		t.Fatalf("encoding not deterministic:\n a=%s\n b=%s", a, b)
	}
}

func TestEntitlementEncodeRejectsBadInput(t *testing.T) {
	_, priv := mustKey(t)

	cases := map[string]func(EntitlementPayload) EntitlementPayload{
		"short tenant id":   func(p EntitlementPayload) EntitlementPayload { p.TenantID = "short"; return p },
		"long voucher id":   func(p EntitlementPayload) EntitlementPayload { p.VoucherID = "way_too_long_id"; return p },
		"empty label":       func(p EntitlementPayload) EntitlementPayload { p.Label = ""; return p },
		"label over 255 b":  func(p EntitlementPayload) EntitlementPayload { p.Label = string(make([]byte, 256)); return p },
		"no lines":          func(p EntitlementPayload) EntitlementPayload { p.Lines = nil; return p },
		"short product id":  func(p EntitlementPayload) EntitlementPayload { p.Lines[0].ProductID = "nope"; return p },
		"zero quantity":     func(p EntitlementPayload) EntitlementPayload { p.Lines[0].Quantity = 0; return p },
		"quantity over 255": func(p EntitlementPayload) EntitlementPayload { p.Lines[0].Quantity = 256; return p },
		"too many lines": func(p EntitlementPayload) EntitlementPayload {
			p.Lines = make([]EntitlementLine, 256)
			for i := range p.Lines {
				p.Lines[i] = EntitlementLine{ProductID: "prod_aaa1111", Quantity: 1}
			}
			return p
		},
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SignEntitlement(priv, mut(sampleEntitlement())); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestEntitlementTamperedFails(t *testing.T) {
	pub, priv := mustKey(t)
	token, err := SignEntitlement(priv, sampleEntitlement())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw[2] ^= 0x01
	if _, err := VerifyEntitlement(pub, base64.RawURLEncoding.EncodeToString(raw)); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestEntitlementTruncatedBodyRejected(t *testing.T) {
	pub, priv := mustKey(t)
	body, err := encodeEntitlement(sampleEntitlement())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, cut := range []int{1, 2, entLineLen} {
		short := append([]byte(nil), body[:len(body)-cut]...)
		token := append(short, ed25519.Sign(priv, short)...)
		if _, err := VerifyEntitlement(pub, base64.RawURLEncoding.EncodeToString(token)); err != ErrBadPayload {
			t.Fatalf("cut %d: expected ErrBadPayload, got %v", cut, err)
		}
	}
}

func TestEntitlementTrailingBytesRejected(t *testing.T) {
	pub, priv := mustKey(t)
	body, err := encodeEntitlement(sampleEntitlement())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	padded := append(append([]byte(nil), body...), 0x00)
	token := append(padded, ed25519.Sign(priv, padded)...)
	if _, err := VerifyEntitlement(pub, base64.RawURLEncoding.EncodeToString(token)); err != ErrBadPayload {
		t.Fatalf("expected ErrBadPayload, got %v", err)
	}
}

func TestEntitlementCrossDiscriminatorRejected(t *testing.T) {
	pub, priv := mustKey(t)

	entitlement, err := SignEntitlement(priv, sampleEntitlement())
	if err != nil {
		t.Fatalf("sign entitlement: %v", err)
	}
	if _, err := Verify(pub, entitlement); err != ErrUnsupportedVersion {
		t.Fatalf("entitlement via Verify: expected ErrUnsupportedVersion, got %v", err)
	}
	if _, err := VerifyVoucher(pub, entitlement); err != ErrUnsupportedVersion {
		t.Fatalf("entitlement via VerifyVoucher: expected ErrUnsupportedVersion, got %v", err)
	}

	pass, err := SignVoucher(priv, sampleVoucher())
	if err != nil {
		t.Fatalf("sign voucher: %v", err)
	}
	if _, err := VerifyEntitlement(pub, pass); err != ErrUnsupportedVersion {
		t.Fatalf("pass via VerifyEntitlement: expected ErrUnsupportedVersion, got %v", err)
	}

	pickup, err := Sign(priv, samplePayload())
	if err != nil {
		t.Fatalf("sign pickup: %v", err)
	}
	if _, err := VerifyEntitlement(pub, pickup); err != ErrUnsupportedVersion {
		t.Fatalf("pickup via VerifyEntitlement: expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestEntitlementWrongPublicKeyFails(t *testing.T) {
	_, priv := mustKey(t)
	otherPub, _ := mustKey(t)
	token, err := SignEntitlement(priv, sampleEntitlement())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := VerifyEntitlement(otherPub, token); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestEntitlementRejectsGarbageToken(t *testing.T) {
	pub, _ := mustKey(t)
	for _, token := range []string{"!!!not-base64!!!", ""} {
		if _, err := VerifyEntitlement(pub, token); err != ErrMalformed {
			t.Fatalf("%q: expected ErrMalformed, got %v", token, err)
		}
	}
}

func TestEntitlementDecodeRejectsDegenerateLines(t *testing.T) {
	pub, priv := mustKey(t)
	body, err := encodeEntitlement(sampleEntitlement())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	countAt := len(body) - entLineLen - 1

	t.Run("zero line count", func(t *testing.T) {
		forged := append([]byte(nil), body[:countAt+1]...)
		forged[countAt] = 0
		token := append(forged, ed25519.Sign(priv, forged)...)
		if _, err := VerifyEntitlement(pub, base64.RawURLEncoding.EncodeToString(token)); err != ErrBadPayload {
			t.Fatalf("expected ErrBadPayload, got %v", err)
		}
	})

	t.Run("zero quantity", func(t *testing.T) {
		forged := append([]byte(nil), body...)
		forged[len(forged)-2] = 0
		token := append(forged, ed25519.Sign(priv, forged)...)
		if _, err := VerifyEntitlement(pub, base64.RawURLEncoding.EncodeToString(token)); err != ErrBadPayload {
			t.Fatalf("expected ErrBadPayload, got %v", err)
		}
	})
}
