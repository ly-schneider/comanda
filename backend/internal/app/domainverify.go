package app

import (
	"context"
	"sync"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/repository"
	"backend/internal/service"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// StartDomainVerifier runs a background ticker that re-checks DNS for
// not-yet-verified custom domains and stamps them verified once both the CNAME
// and the ownership TXT record are in place. It runs on the privileged
// (RLS-bypassing) client so it can iterate domains across all tenants, threading
// each row's tenant into context the way request traffic would.
//
// Disabled (no-op) when no CNAME target is configured, so local runs don't spin.
func StartDomainVerifier(
	lc fx.Lifecycle,
	cfg config.Config,
	priv *database.PrivilegedClient,
	domains repository.TenantDomainRepository,
	domainSvc service.DomainService,
	logger *zap.Logger,
) {
	if cfg.CustomDomain.CNAMETarget == "" {
		logger.Info("custom-domain verifier disabled (no CNAME target configured)")
		return
	}

	interval := cfg.CustomDomain.VerifyInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	runPass := func() {
		rows, err := domains.ListUnverified(ctx, priv.Client)
		if err != nil {
			logger.Warn("domain verifier: list unverified failed", zap.Error(err))
			return
		}
		for _, row := range rows {
			rctx := repository.ContextWithClient(ctx, priv.Client)
			rctx = repository.ContextWithTenantID(rctx, row.TenantID)
			updated, err := domainSvc.VerifyNow(rctx, row.ID)
			if err != nil {
				logger.Warn("domain verifier: check failed",
					zap.String("domainId", row.ID),
					zap.String("hostname", row.Hostname),
					zap.Error(err))
				continue
			}
			if updated.VerifiedAt != nil {
				logger.Info("custom domain verified",
					zap.String("domainId", row.ID),
					zap.String("hostname", row.Hostname),
					zap.String("tenantId", row.TenantID))
			}
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
			logger.Info("custom-domain verifier started", zap.Duration("interval", interval))
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}
