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

// StartPendingOrderSweeper runs a background ticker that reclaims the inventory
// held by dead pending orders — ones whose customer tapped pay but whose payment
// never resolved (closed tab, no terminal webhook). A live payment is never
// reclaimed here: the failure webhook releases explicitly, and this TTL is set
// far longer than any real payment window, so it only ever catches abandonment.
//
// It enumerates stale orders across all tenants on the privileged (RLS-bypassing)
// client, then reclaims each inside its own tenant-scoped tx so a single failure
// doesn't roll back the rest. Cleanup is idempotent and row-locked, so multiple
// pods sweeping concurrently is safe.
//
// Disabled (no-op) when the interval or TTL is non-positive.
func StartPendingOrderSweeper(
	lc fx.Lifecycle,
	cfg config.Config,
	priv *database.PrivilegedClient,
	appDB *database.AppClient,
	orders repository.OrderRepository,
	payments service.PaymentService,
	logger *zap.Logger,
) {
	ttl := cfg.Inventory.PendingOrderTTL
	interval := cfg.Inventory.PendingSweepInterval
	if interval <= 0 || ttl <= 0 {
		logger.Info("pending-order sweeper disabled (non-positive interval or TTL)")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	runPass := func() {
		cutoff := time.Now().Add(-ttl)
		stale, err := orders.ListStalePending(ctx, priv.Client, cutoff)
		if err != nil {
			logger.Warn("pending sweeper: list stale failed", zap.Error(err))
			return
		}
		var reclaimed int
		for _, o := range stale {
			err := repository.RunInTenantTx(ctx, appDB.DB, o.TenantID, func(fctx context.Context) error {
				return payments.CleanupPendingOrderByID(fctx, o.ID)
			})
			if err != nil {
				logger.Warn("pending sweeper: cleanup failed",
					zap.String("orderId", o.ID),
					zap.String("tenantId", o.TenantID),
					zap.Error(err),
				)
				continue
			}
			reclaimed++
		}
		if reclaimed > 0 {
			logger.Info("reclaimed stale pending orders", zap.Int("count", reclaimed))
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
			logger.Info("pending-order sweeper started",
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
