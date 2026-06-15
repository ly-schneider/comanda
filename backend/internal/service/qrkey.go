package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"

	"backend/internal/crypto"
	"backend/internal/repository"
)

// QRKeyService manages the per-tenant Ed25519 keypair that signs offline-
// verifiable pickup QR tokens.
type QRKeyService interface {
	// EnsureTenantKey returns the tenant's signing key, generating and persisting
	// one on first use. Idempotent: repeated calls return the same key.
	EnsureTenantKey(ctx context.Context) (ed25519.PrivateKey, ed25519.PublicKey, error)
}

type qrKeyService struct {
	settings  repository.SettingsRepository
	encryptor *crypto.Encryptor
}

func NewQRKeyService(settings repository.SettingsRepository, encryptor *crypto.Encryptor) QRKeyService {
	return &qrKeyService{settings: settings, encryptor: encryptor}
}

func (s *qrKeyService) EnsureTenantKey(ctx context.Context) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	if s.encryptor == nil {
		return nil, nil, fmt.Errorf("qr signing key: encryptor not configured")
	}

	existing, err := s.settings.GetQRSigningKey(ctx)
	if err == nil {
		return s.deriveFromStored(existing)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, nil, err
	}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("qr signing key: generate: %w", err)
	}

	seedEnc, err := s.encryptor.Encrypt(priv.Seed())
	if err != nil {
		return nil, nil, fmt.Errorf("qr signing key: encrypt seed: %w", err)
	}
	pubB64 := base64.RawURLEncoding.EncodeToString(pub)

	if err := s.settings.UpsertQRSigningKey(ctx, repository.QRSigningKey{
		PrivateSeedEnc: seedEnc,
		PublicKey:      pubB64,
	}); err != nil {
		return nil, nil, fmt.Errorf("qr signing key: persist: %w", err)
	}

	// Re-read so we return whichever key actually landed in the row: a racing
	// caller may have won the COALESCE upsert with a different key.
	stored, err := s.settings.GetQRSigningKey(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("qr signing key: reload: %w", err)
	}
	return s.deriveFromStored(stored)
}

func (s *qrKeyService) deriveFromStored(k *repository.QRSigningKey) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	seed, err := s.encryptor.Decrypt(k.PrivateSeedEnc)
	if err != nil {
		return nil, nil, fmt.Errorf("qr signing key: decrypt seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("qr signing key: bad seed length %d", len(seed))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("qr signing key: unexpected public key type")
	}
	return priv, pub, nil
}
