package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"
)

func newTestEncryptor(t *testing.T) *Encryptor {
	t.Helper()
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("read key: %v", err)
	}
	e, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}
	return e
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	e := newTestEncryptor(t)
	for _, pt := range []string{"", "secret", "api_secret_with_unicode_äöü", "x" + string(make([]byte, 4096))} {
		blob, err := e.EncryptString(pt)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		got, err := e.DecryptString(blob)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if got != pt {
			t.Fatalf("round-trip mismatch: got %q want %q", got, pt)
		}
		if blob == pt && pt != "" {
			t.Fatalf("ciphertext equals plaintext for %q", pt)
		}
	}
}

func TestInvalidKeyLength(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := NewEncryptor(make([]byte, n)); err != ErrInvalidKey {
			t.Fatalf("key len %d: expected ErrInvalidKey, got %v", n, err)
		}
	}
}

func TestTamperedCiphertextFails(t *testing.T) {
	e := newTestEncryptor(t)
	blob, err := e.EncryptString("webhook_secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(blob)
	raw[len(raw)-1] ^= 0xFF // flip a tag bit
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := e.Decrypt(tampered); err != ErrMalformedCiphertext {
		t.Fatalf("expected ErrMalformedCiphertext, got %v", err)
	}
}

func TestWrongKeyFails(t *testing.T) {
	a := newTestEncryptor(t)
	b := newTestEncryptor(t)
	blob, err := a.EncryptString("instance_secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := b.Decrypt(blob); err != ErrMalformedCiphertext {
		t.Fatalf("expected ErrMalformedCiphertext decrypting under wrong key, got %v", err)
	}
}

func TestNonceUniqueness(t *testing.T) {
	e := newTestEncryptor(t)
	const n = 100
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		blob, err := e.EncryptString("same-plaintext")
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		if _, dup := seen[blob]; dup {
			t.Fatalf("duplicate ciphertext at iteration %d (nonce reuse)", i)
		}
		seen[blob] = struct{}{}
	}
}

func TestDecryptGarbageFails(t *testing.T) {
	e := newTestEncryptor(t)
	for _, bad := range []string{"not-base64-!!!", "", "YQ=="} {
		if _, err := e.Decrypt(bad); err != ErrMalformedCiphertext {
			t.Fatalf("decrypt %q: expected ErrMalformedCiphertext, got %v", bad, err)
		}
	}
}
