// Package crypto provides AES-256-GCM encryption for per-tenant provider
// credentials. The master key is supplied out-of-band and never persisted;
// only ciphertext is stored.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const KeySize = 32

var (
	ErrInvalidKey          = errors.New("crypto: master key must be 32 bytes")
	ErrMalformedCiphertext = errors.New("crypto: malformed ciphertext")
)

// Encryptor seals and opens secrets with AES-256-GCM. The random nonce is
// prepended to the ciphertext (not secret); the blob is base64-encoded.
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor fails closed on a wrong-length key so a misconfigured deployment
// never silently runs without real encryption.
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &Encryptor{gcm: gcm}, nil
}

func NewEncryptorFromBase64(b64 string) (*Encryptor, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode master key: %w", err)
	}
	return NewEncryptor(key)
}

func (e *Encryptor) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	sealed := e.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (e *Encryptor) EncryptString(plaintext string) (string, error) {
	return e.Encrypt([]byte(plaintext))
}

func (e *Encryptor) Decrypt(blob string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, ErrMalformedCiphertext
	}
	ns := e.gcm.NonceSize()
	if len(raw) < ns {
		return nil, ErrMalformedCiphertext
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrMalformedCiphertext
	}
	return plaintext, nil
}

func (e *Encryptor) DecryptString(blob string) (string, error) {
	b, err := e.Decrypt(blob)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
