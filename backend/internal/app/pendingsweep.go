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
// Release is destructive (the order row goes away), so each candidate is first
// verified against the payment provider. An order that turns out to be paid is
// settled instead of released — the last chance to rescue a customer whose
// webhook never arrived.
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
		var reclaimed, settled int
		for _, o := range stale {
			var wasPaid bool
			err := repository.RunInTenantTx(ctx, appDB.DB, o.TenantID, func(fctx context.Context) error {
				// Releasing deletes the order, which would strand a payment that
				// did go through, so ask the provider first. A provider that can't
				// answer means we don't know — the error keeps the hold and we
				// retry next pass rather than delete a possibly-paid order.
				res, err := payments.ReconcilePendingOrder(fctx, o.ID)
				if err != nil {
					return err
				}
				if res.Paid {
					wasPaid = true
					return nil
				}
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
			if wasPaid {
				settled++
				continue
			}
			reclaimed++
		}
		if reclaimed > 0 || settled > 0 {
			logger.Info("swept stale pending orders",
				zap.Int("reclaimed", reclaimed),
				zap.Int("settledInstead", settled),
			)
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
