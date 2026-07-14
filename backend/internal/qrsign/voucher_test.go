package qrsign

import (
	"encoding/base64"
	"encoding/hex"
	"reflect"
	"testing"
)

// Canonical cross-language golden vector for the 0x10 voucher token. The PDF
// generator and the web/Android readers reproduce these exact bytes; any
// change here is a coordinated wire-format change for every reader.
const (
	goldenVoucherBodyHex = "1074656e616e745f73656564315663683741623243643945666553f10065554280094c6976652042616e6402064472696e6b7304466f6f64"
	goldenVoucherToken   = "EHRlbmFudF9zZWVkMVZjaDdBYjJDZDlFZmVT8QBlVUKACUxpdmUgQmFuZAIGRHJpbmtzBEZvb2S8kQe39D7m2BieSGN4fzWqDHyN18POcyqGizisVN_jU_bQ3r3EMF-OkMRl9WSPaAhWksbXZX7nOXKY_vgLgxYL"
)

func goldenVoucher() VoucherPayload {
	return VoucherPayload{
		TenantID:   "tenant_seed1",
		VoucherID:  "Vch7Ab2Cd9Ef",
		ValidFrom:  1_700_000_000,
		ValidUntil: 1_700_086_400,
		Label:      "Live Band",
		Categories: []string{"Drinks", "Food"},
	}
}

func sampleVoucher() VoucherPayload {
	return VoucherPayload{
		TenantID:   "tenant_abc12",
		VoucherID:  "voucher_xyz9",
		ValidFrom:  1_700_000_000,
		ValidUntil: 1_700_100_000,
		Label:      "VIP Guest",
		Categories: []string{"Beer", "Wine", "Snacks"},
	}
}

func TestVoucherGoldenVector(t *testing.T) {
	pub, priv := goldenKey(t)

	body, err := encodeVoucher(goldenVoucher())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := hex.EncodeToString(body); got != goldenVoucherBodyHex {
		t.Fatalf("body drift:\n got %s\nwant %s", got, goldenVoucherBodyHex)
	}

	token, err := SignVoucher(priv, goldenVoucher())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if token != goldenVoucherToken {
		t.Fatalf("token drift:\n got %s\nwant %s", token, goldenVoucherToken)
	}

	got, err := VerifyVoucher(pub, goldenVoucherToken)
	if err != nil {
		t.Fatalf("verify golden: %v", err)
	}
	if !reflect.DeepEqual(got, goldenVoucher()) {
		t.Fatalf("golden payload mismatch:\n got %+v\nwant %+v", got, goldenVoucher())
	}
}

func TestVoucherRoundTrip(t *testing.T) {
	pub, priv := mustKey(t)
	p := sampleVoucher()

	token, err := SignVoucher(priv, p)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := VerifyVoucher(pub, token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Fatalf("payload mismatch:\n got %+v\nwant %+v", got, p)
	}
}

func TestVoucherRoundTripNoCategories(t *testing.T) {
	pub, priv := mustKey(t)
	p := sampleVoucher()
	p.Categories = nil

	token, err := SignVoucher(priv, p)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := VerifyVoucher(pub, token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(got.Categories) != 0 {
		t.Fatalf("expected 0 categories, got %d", len(got.Categories))
	}
}

func TestVoucherDeterministicEncoding(t *testing.T) {
	_, priv := mustKey(t)
	p := sampleVoucher()
	a, err := SignVoucher(priv, p)
	if err != nil {
		t.Fatalf("sign a: %v", err)
	}
	b, err := SignVoucher(priv, p)
	if err != nil {
		t.Fatalf("sign b: %v", err)
	}
	if a != b {
		t.Fatalf("encoding not deterministic:\n a=%s\n b=%s", a, b)
	}
}

func TestVoucherEncodeRejectsBadIDLength(t *testing.T) {
	_, priv := mustKey(t)
	cases := []VoucherPayload{
		{TenantID: "short", VoucherID: "voucher_xyz9", Label: "x", ValidUntil: 1},
		{TenantID: "tenant_abc12", VoucherID: "too_long_voucher_id", Label: "x", ValidUntil: 1},
	}
	for i, p := range cases {
		if _, err := SignVoucher(priv, p); err == nil {
			t.Fatalf("case %d: expected error for bad id length", i)
		}
	}
}

func TestVoucherEncodeRejectsEmptyLabel(t *testing.T) {
	_, priv := mustKey(t)
	p := sampleVoucher()
	p.Label = ""
	if _, err := SignVoucher(priv, p); err == nil {
		t.Fatal("expected error for empty label")
	}
}

func TestVoucherEncodeRejectsTooLongLabel(t *testing.T) {
	_, priv := mustKey(t)
	p := sampleVoucher()
	p.Label = string(make([]byte, 256))
	if _, err := SignVoucher(priv, p); err == nil {
		t.Fatal("expected error for label > 255 bytes")
	}
}

func TestVoucherEncodeRejectsTooManyCategories(t *testing.T) {
	_, priv := mustKey(t)
	p := sampleVoucher()
	p.Categories = make([]string, 256)
	for i := range p.Categories {
		p.Categories[i] = "x"
	}
	if _, err := SignVoucher(priv, p); err == nil {
		t.Fatal("expected error for > 255 categories")
	}
}

func TestVoucherTamperedFails(t *testing.T) {
	pub, priv := mustKey(t)
	token, err := SignVoucher(priv, sampleVoucher())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw[2] ^= 0x01
	if _, err := VerifyVoucher(pub, base64.RawURLEncoding.EncodeToString(raw)); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

// A 0x03 pickup token must not decode as a voucher, and a 0x10 voucher token
// must not decode as a pickup token — the discriminator keeps them disjoint.
func TestVoucherCrossDiscriminatorRejected(t *testing.T) {
	pub, priv := mustKey(t)

	pickup, err := Sign(priv, samplePayload())
	if err != nil {
		t.Fatalf("sign pickup: %v", err)
	}
	if _, err := VerifyVoucher(pub, pickup); err != ErrUnsupportedVersion {
		t.Fatalf("pickup token via VerifyVoucher: expected ErrUnsupportedVersion, got %v", err)
	}

	voucher, err := SignVoucher(priv, sampleVoucher())
	if err != nil {
		t.Fatalf("sign voucher: %v", err)
	}
	if _, err := Verify(pub, voucher); err != ErrUnsupportedVersion {
		t.Fatalf("voucher token via Verify: expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestVoucherWrongPublicKeyFails(t *testing.T) {
	_, priv := mustKey(t)
	otherPub, _ := mustKey(t)
	token, err := SignVoucher(priv, sampleVoucher())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := VerifyVoucher(otherPub, token); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestVoucherRejectsGarbageToken(t *testing.T) {
	pub, _ := mustKey(t)
	if _, err := VerifyVoucher(pub, "!!!not-base64!!!"); err != ErrMalformed {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}
