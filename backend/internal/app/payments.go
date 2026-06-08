package app

import (
	"backend/internal/config"
	"backend/internal/crypto"
	"backend/internal/payment"
	"backend/internal/repository"

	"go.uber.org/zap"
)

// NewEncryptor returns a nil encryptor when no master key is set (provider use
// then fails closed), but treats a set-but-invalid key as a fatal error.
func NewEncryptor(cfg config.Config, logger *zap.Logger) (*crypto.Encryptor, error) {
	if cfg.Payments.MasterKey == "" {
		logger.Warn("PAYMENTS_MASTER_KEY not set; per-tenant payment/email credentials are disabled")
		return nil, nil
	}
	enc, err := crypto.NewEncryptorFromBase64(cfg.Payments.MasterKey)
	if err != nil {
		return nil, err
	}
	logger.Info("payments encryptor initialized")
	return enc, nil
}

func NewPaymentFactory(settings repository.SettingsRepository, enc *crypto.Encryptor) *payment.Factory {
	return payment.NewFactory(settings, enc)
}
