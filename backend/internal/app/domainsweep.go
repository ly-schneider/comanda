package app

import (
	"context"
	"sync"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/repository"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// StartUnverifiedDomainSweeper runs a background ticker that drops BYO custom
// domains left unverified past the configured TTL. Because tenant_domain.hostname
// is globally unique, a squatted-but-unverified registration would otherwise block
// the rightful owner from ever registering it; sweeping stale rows frees the name.
// It runs on the privileged (RLS-bypassing) client and only removes rows that
// carry a verification token, so platform-assigned domains are never touched.
//
// Disabled (no-op) when the verification pipeline is off (no CNAME target) or the
// TTL is non-positive — otherwise it could delete domains that can never verify.
func StartUnverifiedDomainSweeper(
	lc fx.Lifecycle,
	cfg config.Config,
	priv *database.PrivilegedClient,
	domains repository.TenantDomainRepository,
	logger *zap.Logger,
) {
	ttl := cfg.CustomDomain.UnverifiedTTL
	if cfg.CustomDomain.CNAMETarget == "" || ttl <= 0 {
		logger.Info("unverified-domain sweeper disabled (no CNAME target or non-positive TTL)")
		return
	}

	interval := cfg.CustomDomain.SweepInterval
	if interval <= 0 {
		interval = time.Hour
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	runPass := func() {
		cutoff := time.Now().Add(-ttl)
		n, err := domains.DeleteStaleUnverified(ctx, priv.Client, cutoff)
		if err != nil {
			logger.Warn("domain sweeper: delete stale unverified failed", zap.Error(err))
			return
		}
		if n > 0 {
			logger.Info("swept stale unverified custom domains", zap.Int("count", n))
		}
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				runPass()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						runPass()
					}
				}
			}()
			logger.Info("unverified-domain sweeper started",
				zap.Duration("interval", interval),
				zap.Duration("ttl", ttl))
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}
