package payment

import (
	"context"
	"errors"
	"fmt"

	"backend/internal/crypto"
	"backend/internal/payrexx"
	"backend/internal/repository"
)

var ErrNotConfigured = errors.New("payment: provider not configured for tenant")

type Factory struct {
	settings  repository.SettingsRepository
	encryptor *crypto.Encryptor
}

func NewFactory(settings repository.SettingsRepository, encryptor *crypto.Encryptor) *Factory {
	return &Factory{settings: settings, encryptor: encryptor}
}

func (f *Factory) HasEncryptor() bool { return f.encryptor != nil }

// For requires the caller to run inside the tenant's transaction (app.tenant_id
// set) so the RLS-scoped settings read returns this tenant's row.
func (f *Factory) For(ctx context.Context) (Provider, error) {
	if f.encryptor == nil {
		return nil, ErrNotConfigured
	}
	creds, err := f.settings.GetPayrexxCredentials(ctx)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotConfigured
		}
		return nil, err
	}
	apiSecret, err := f.encryptor.DecryptString(creds.APISecretEnc)
	if err != nil {
		return nil, fmt.Errorf("payment: decrypt api secret: %w", err)
	}
	webhookSecret, err := f.encryptor.DecryptString(creds.WebhookSecretEnc)
	if err != nil {
		return nil, fmt.Errorf("payment: decrypt webhook secret: %w", err)
	}
	return NewPayrexxProvider(creds.Instance, apiSecret, webhookSecret), nil
}

func (f *Factory) IsConfigured(ctx context.Context) bool {
	if f.encryptor == nil {
		return false
	}
	_, err := f.settings.GetPayrexxCredentials(ctx)
	return err == nil
}

// ValidatePayrexxCredentials makes a live Payrexx call to confirm the
// instance/api-secret pair before they are encrypted and stored.
func ValidatePayrexxCredentials(ctx context.Context, instance, apiSecret string) error {
	client := payrexx.NewClient(instance, apiSecret)
	if err := client.ValidateCredentials(ctx); err != nil {
		return fmt.Errorf("payment: payrexx credential validation failed: %w", err)
	}
	return nil
}
