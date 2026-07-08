package qrsign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// Canonical cross-language golden vectors. The web (@noble/ed25519) and Android
// (ZXing byte-mode) tests reproduce these exact bytes; if any of them changes,
// every reader is affected and the change is a coordinated wire-format bump.
const (
	goldenSeedHex   = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	goldenPubB64Url = "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg"
	goldenBodyHex   = "0274656e616e745f73656564314853373955317948375a64336553f1000270726f645f706f6d6d6573310270726f645f636f6c6130303101"
	goldenToken     = "AnRlbmFudF9zZWVkMUhTNzlVMXlIN1pkM2VT8QACcHJvZF9wb21tZXMxAnByb2RfY29sYTAwMQHaMq10Re5WTXw5EhrtgeLUbwYKDAj0e4J3qslI-xRrgyJcC60lyKkgDqs911g5tvZRWQnA2B2SGyhK1G_xckUE"
)

func goldenPayload() Payload {
	return Payload{
		Version:  Version,
		TenantID: "tenant_seed1",
		OrderID:  "HS79U1yH7Zd3",
		IssuedAt: 1_700_000_000,
		Lines: []Line{
			{ProductID: "prod_pommes1", Quantity: 2},
			{ProductID: "prod_cola001", Quantity: 1},
		},
	}
}

func goldenKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed, err := hex.DecodeString(goldenSeedHex)
	if err != nil {
		t.Fatalf("decode seed: %v", err)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return priv.Public().(ed25519.PublicKey), priv
}

func samplePayload() Payload {
	return Payload{
		Version:  Version,
		TenantID: "tenant_abc12",
		OrderID:  "order_xyz789",
		IssuedAt: 1_700_000_000,
		Lines: []Line{
			{ProductID: "prod_aaa1111", Quantity: 2},
			{ProductID: "prod_bbb2222", Quantity: 1},
		},
	}
}

func mustKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

// TestGoldenVector pins the exact byte layout the web/Android readers mirror.
func TestGoldenVector(t *testing.T) {
	pub, priv := goldenKey(t)

	if got := base64.RawURLEncoding.EncodeToString(pub); got != goldenPubB64Url {
		t.Fatalf("public key drift: got %s want %s", got, goldenPubB64Url)
	}

	body, err := encodeV2(goldenPayload())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := hex.EncodeToString(body); got != goldenBodyHex {
		t.Fatalf("body drift:\n got %s\nwant %s", got, goldenBodyHex)
	}

	token, err := Sign(priv, goldenPayload())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if token != goldenToken {
		t.Fatalf("token drift:\n got %s\nwant %s", token, goldenToken)
	}

	got, err := Verify(pub, goldenToken)
	if err != nil {
		t.Fatalf("verify golden: %v", err)
	}
	if got.Version != 2 || got.TenantID != "tenant_seed1" || got.OrderID != "HS79U1yH7Zd3" || got.IssuedAt != 1_700_000_000 {
		t.Fatalf("golden payload mismatch: %+v", got)
	}
	if len(got.Lines) != 2 || got.Lines[0] != (Line{"prod_pommes1", 2}) || got.Lines[1] != (Line{"prod_cola001", 1}) {
		t.Fatalf("golden lines mismatch: %+v", got.Lines)
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	pub, priv := mustKey(t)
	p := samplePayload()

	token, err := Sign(priv, p)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	got, err := Verify(pub, token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Version != Version || got.TenantID != p.TenantID || got.OrderID != p.OrderID || got.IssuedAt != p.IssuedAt {
		t.Fatalf("payload mismatch: got %+v want %+v", got, p)
	}
	if len(got.Lines) != len(p.Lines) {
		t.Fatalf("lines len mismatch: got %d want %d", len(got.Lines), len(p.Lines))
	}
	for i := range p.Lines {
		if got.Lines[i] != p.Lines[i] {
			t.Fatalf("line %d mismatch: got %+v want %+v", i, got.Lines[i], p.Lines[i])
		}
	}
}

func TestBodyLayout(t *testing.T) {
	_, priv := mustKey(t)
	token, err := Sign(priv, samplePayload())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 2 lines: header(30) + 2*13 + sig(64) = 120.
	if len(raw) != headerLen+2*lineLen+ed25519.SignatureSize {
		t.Fatalf("unexpected token length: %d", len(raw))
	}
	if raw[0] != verV2 {
		t.Fatalf("first byte = %#x, want v2 discriminator %#x", raw[0], verV2)
	}
}

func TestDeterministicEncoding(t *testing.T) {
	_, priv := mustKey(t)
	p := samplePayload()
	a, err := Sign(priv, p)
	if err != nil {
		t.Fatalf("sign a: %v", err)
	}
	b, err := Sign(priv, p)
	if err != nil {
		t.Fatalf("sign b: %v", err)
	}
	if a != b {
		t.Fatalf("encoding not deterministic:\n a=%s\n b=%s", a, b)
	}
}

func TestTamperedPayloadFails(t *testing.T) {
	pub, priv := mustKey(t)
	token, err := Sign(priv, samplePayload())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Flip a bit inside the body (a tenant-id byte, not the signature).
	raw[2] ^= 0x01
	tampered := base64.RawURLEncoding.EncodeToString(raw)

	if _, err := Verify(pub, tampered); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestWrongPublicKeyFails(t *testing.T) {
	_, priv := mustKey(t)
	otherPub, _ := mustKey(t)
	token, err := Sign(priv, samplePayload())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := Verify(otherPub, token); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestGarbageToken(t *testing.T) {
	pub, _ := mustKey(t)
	if _, err := Verify(pub, "!!!not-base64!!!"); err != ErrMalformed {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestTruncatedToken(t *testing.T) {
	pub, _ := mustKey(t)
	short := base64.RawURLEncoding.EncodeToString([]byte("tooshort"))
	if _, err := Verify(pub, short); err != ErrMalformed {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestExactlySigSizeToken(t *testing.T) {
	pub, _ := mustKey(t)
	// 64 bytes decoded = signature with an empty body; rejected as malformed
	// before reaching signature/parse.
	exact := base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if _, err := Verify(pub, exact); err != ErrMalformed {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

// signRaw is a test helper: sign an arbitrary body so we can exercise Verify's
// discriminator branches that Sign itself would never emit.
func signRaw(priv ed25519.PrivateKey, body []byte) string {
	sig := ed25519.Sign(priv, body)
	return base64.RawURLEncoding.EncodeToString(append(append([]byte{}, body...), sig...))
}

func TestVerifyRejectsUnknownVersion(t *testing.T) {
	pub, priv := mustKey(t)
	// A validly-signed body whose first byte is neither v2 (0x02) nor v1 ('{').
	token := signRaw(priv, []byte{0x09, 0xAA, 0xBB})
	if _, err := Verify(pub, token); err != ErrUnsupportedVersion {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestVerifyRejectsRetiredV1JSON(t *testing.T) {
	pub, priv := mustKey(t)
	// A validly-signed legacy v1 JSON body starts with '{' (0x7B), not 0x02, so
	// the v2-only verifier must reject it as an unsupported version.
	v1Body := []byte(`{"v":1,"t":"tenant_abc12","o":"order_xyz789","i":1700000000,"l":[{"p":"prod_aaa1111","q":3}]}`)
	if _, err := Verify(pub, signRaw(priv, v1Body)); err != ErrUnsupportedVersion {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestEncodeRejectsBadIDLength(t *testing.T) {
	_, priv := mustKey(t)
	cases := []Payload{
		{TenantID: "short", OrderID: "order_xyz789", Lines: []Line{{ProductID: "prod_aaa1111", Quantity: 1}}},
		{TenantID: "tenant_abc12", OrderID: "toolong_order_id", Lines: []Line{{ProductID: "prod_aaa1111", Quantity: 1}}},
		{TenantID: "tenant_abc12", OrderID: "order_xyz789", Lines: []Line{{ProductID: "short", Quantity: 1}}},
	}
	for i, p := range cases {
		if _, err := Sign(priv, p); err == nil {
			t.Fatalf("case %d: expected error for bad id length", i)
		}
	}
}

func TestEncodeRejectsBadQuantity(t *testing.T) {
	_, priv := mustKey(t)
	for _, qty := range []int{0, 256, -1} {
		p := samplePayload()
		p.Lines = []Line{{ProductID: "prod_aaa1111", Quantity: qty}}
		if _, err := Sign(priv, p); err == nil {
			t.Fatalf("qty %d: expected error", qty)
		}
	}
}

func TestEncodeRejectsTooManyLines(t *testing.T) {
	_, priv := mustKey(t)
	p := samplePayload()
	p.Lines = make([]Line, 256)
	for i := range p.Lines {
		p.Lines[i] = Line{ProductID: "prod_aaa1111", Quantity: 1}
	}
	if _, err := Sign(priv, p); err == nil {
		t.Fatal("expected error for >255 lines")
	}
}

func TestWrongLengthPublicKey(t *testing.T) {
	_, priv := mustKey(t)
	token, err := Sign(priv, samplePayload())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := Verify(ed25519.PublicKey([]byte("short")), token); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestZeroLineOrder(t *testing.T) {
	pub, priv := mustKey(t)
	p := samplePayload()
	p.Lines = nil
	token, err := Sign(priv, p)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := Verify(pub, token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(got.Lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(got.Lines))
	}
}
