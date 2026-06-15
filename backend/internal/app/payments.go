package app

import (
	"fmt"

	"backend/internal/config"
	"backend/internal/crypto"
	"backend/internal/payment"
	"backend/internal/repository"

	"go.uber.org/zap"
)

// NewEncryptor builds the AES-256-GCM encryptor from the master key. The key is
// mandatory and the app refuses to start without it: besides per-tenant
// payment/email credentials, it protects the Ed25519 QR signing seeds that every
// order is signed with, so a missing key would fail all order creation at request
// time. Fail fast at boot instead.
func NewEncryptor(cfg config.Config, logger *zap.Logger) (*crypto.Encryptor, error) {
	if cfg.Payments.MasterKey == "" {
		return nil, fmt.Errorf("PAYMENTS_MASTER_KEY is required: it encrypts payment/email credentials and the per-tenant QR signing keys")
	}
	enc, err := crypto.NewEncryptorFromBase64(cfg.Payments.MasterKey)
	if err != nil {
		return nil, fmt.Errorf("invalid PAYMENTS_MASTER_KEY: %w", err)
	}
	logger.Info("payments encryptor initialized")
	return enc, nil
}

func NewPaymentFactory(settings repository.SettingsRepository, enc *crypto.Encryptor) *payment.Factory {
	return payment.NewFactory(settings, enc)
}
