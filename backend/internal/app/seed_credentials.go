package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"backend/internal/config"
	"backend/internal/crypto"
	"backend/internal/database"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/settings"
	"backend/internal/generated/ent/tenant"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// SeedProviderCredentials is an idempotent first-boot migration of legacy global
// Payrexx/Plunk env onto the bootstrap tenant's encrypted settings row; a no-op
// when unset or already seeded. It uses the privileged client because it runs at
// boot with no tenant context.
func SeedProviderCredentials(
	lc fx.Lifecycle,
	cfg config.Config,
	priv *database.PrivilegedClient,
	enc *crypto.Encryptor,
	logger *zap.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if enc == nil {
				return nil
			}
			havePayrexx := cfg.Payrexx.InstanceName != "" && cfg.Payrexx.APISecret != "" && cfg.Payrexx.WebhookSecret != ""
			havePlunk := cfg.Plunk.APIKey != ""
			if !havePayrexx && !havePlunk {
				return nil
			}
			if cfg.App.BootstrapTenantSlug == "" {
				return nil
			}

			t, err := priv.Client.Tenant.Query().
				Where(tenant.SlugEQ(cfg.App.BootstrapTenantSlug)).
				Only(ctx)
			if err != nil {
				if ent.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("seed credentials: lookup bootstrap tenant: %w", err)
			}
			id := t.ID

			row, err := priv.Client.Settings.Get(ctx, id)
			if err != nil && !ent.IsNotFound(err) {
				return fmt.Errorf("seed credentials: load settings: %w", err)
			}

			create := priv.Client.Settings.Create().SetID(id).SetTenantID(t.ID)
			update := func(u *ent.SettingsUpsert) {}

			var (
				setPayrexx bool
				setPlunk   bool
				token      string
			)

			if havePayrexx && (row == nil || row.PayrexxAPISecretEnc == nil) {
				apiEnc, e1 := enc.EncryptString(cfg.Payrexx.APISecret)
				whEnc, e2 := enc.EncryptString(cfg.Payrexx.WebhookSecret)
				if e1 != nil || e2 != nil {
					return fmt.Errorf("seed credentials: encrypt payrexx: %v / %v", e1, e2)
				}
				token = ""
				if row != nil && row.PaymentWebhookToken != nil {
					token = *row.PaymentWebhookToken
				}
				if token == "" {
					token, err = seedToken()
					if err != nil {
						return err
					}
				}
				create = create.
					SetPayrexxInstance(cfg.Payrexx.InstanceName).
					SetPayrexxAPISecretEnc(apiEnc).
					SetPayrexxWebhookSecretEnc(whEnc).
					SetPaymentWebhookToken(token).
					SetPaymentProvider("payrexx")
				prevUpdate := update
				update = func(u *ent.SettingsUpsert) {
					prevUpdate(u)
					u.SetPayrexxInstance(cfg.Payrexx.InstanceName)
					u.SetPayrexxAPISecretEnc(apiEnc)
					u.SetPayrexxWebhookSecretEnc(whEnc)
					u.SetPaymentWebhookToken(token)
					u.SetPaymentProvider("payrexx")
				}
				setPayrexx = true
			}

			if havePlunk && (row == nil || row.PlunkAPIKeyEnc == nil) {
				keyEnc, e := enc.EncryptString(cfg.Plunk.APIKey)
				if e != nil {
					return fmt.Errorf("seed credentials: encrypt plunk: %w", e)
				}
				create = create.
					SetPlunkAPIKeyEnc(keyEnc).
					SetPlunkFromName(cfg.Plunk.FromName).
					SetPlunkFromEmail(cfg.Plunk.FromEmail).
					SetPlunkReplyTo(cfg.Plunk.ReplyTo)
				prevUpdate := update
				update = func(u *ent.SettingsUpsert) {
					prevUpdate(u)
					u.SetPlunkAPIKeyEnc(keyEnc)
					u.SetPlunkFromName(cfg.Plunk.FromName)
					u.SetPlunkFromEmail(cfg.Plunk.FromEmail)
					u.SetPlunkReplyTo(cfg.Plunk.ReplyTo)
				}
				setPlunk = true
			}

			if !setPayrexx && !setPlunk {
				return nil
			}

			if err := create.
				OnConflictColumns(settings.FieldID).
				Update(update).
				Exec(ctx); err != nil {
				return fmt.Errorf("seed credentials: upsert: %w", err)
			}
			logger.Info("seeded bootstrap tenant provider credentials from global env",
				zap.Bool("payrexx", setPayrexx), zap.Bool("plunk", setPlunk))
			return nil
		},
	})
}

func seedToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
