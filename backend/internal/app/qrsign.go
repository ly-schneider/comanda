package app

import (
	"context"
	"fmt"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenant"
	"backend/internal/repository"
	"backend/internal/service"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// EnsureBootstrapQRKey provisions the bootstrap tenant's Ed25519 QR signing key
// at boot so GET /v1/qr-config is a pure read instead of generating a key as a
// GET side effect. Idempotent: a no-op once a key exists, and a no-op when the
// bootstrap tenant isn't seeded yet (the first order then provisions it lazily).
// Runs on the privileged client with no request tx, so the tenant is threaded in
// explicitly.
func EnsureBootstrapQRKey(
	lc fx.Lifecycle,
	cfg config.Config,
	priv *database.PrivilegedClient,
	qrKeys service.QRKeyService,
	logger *zap.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
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
				return fmt.Errorf("ensure qr key: lookup bootstrap tenant: %w", err)
			}

			kctx := repository.ContextWithClient(ctx, priv.Client)
			kctx = repository.ContextWithTenantID(kctx, t.ID)
			if _, _, err := qrKeys.EnsureTenantKey(kctx); err != nil {
				return fmt.Errorf("ensure qr key: %w", err)
			}
			logger.Info("bootstrap tenant QR signing key ensured", zap.String("tenant", t.ID))
			return nil
		},
	})
}
