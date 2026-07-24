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

// StartPaymentReconciler asks the payment provider about orders whose payment
// never resolved locally, and settles the ones that actually went through.
//
// The webhook is a single push at our API: delayed, dropped, or delivered while
// a pod was rolling, it leaves a customer who has paid without a pickup QR and
// with no way to get one. This pass makes confirmation pull-based as well, so
// the customer's order appears on its own — whether or not they ever came back
// from the TWINT app.
//
// It enumerates candidates across all tenants on the privileged (RLS-bypassing)
// client, then reconciles each inside its own tenant-scoped tx so one failure
// doesn't roll back the rest. Reconciling is idempotent and row-locked, so
// several pods running this concurrently is safe.
//
// Disabled (no-op) when the interval is non-positive.
func StartPaymentReconciler(
	lc fx.Lifecycle,
	cfg config.Config,
	priv *database.PrivilegedClient,
	appDB *database.AppClient,
	orders repository.OrderRepository,
	payments service.PaymentService,
	logger *zap.Logger,
) {
	rc := cfg.PaymentSync
	if rc.Interval <= 0 {
		logger.Info("payment reconciler disabled (non-positive interval)")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	runPass := func() {
		now := time.Now()
		candidates, err := orders.ListPendingForReconcile(ctx, priv.Client,
			now.Add(-rc.MinAge), now.Add(-rc.MaxAge), rc.BatchSize)
		if err != nil {
			logger.Warn("payment reconciler: list candidates failed", zap.Error(err))
			return
		}
		if len(candidates) == rc.BatchSize && rc.BatchSize > 0 {
			// The window held more orders than one pass polls; the remainder is
			// picked up next pass. Say so rather than looking fully covered.
			logger.Info("payment reconciler: batch full, deferring remainder",
				zap.Int("batchSize", rc.BatchSize))
		}

		var settled int
		for _, o := range candidates {
			err := repository.RunInTenantTx(ctx, appDB.DB, o.TenantID, func(fctx context.Context) error {
				res, err := payments.ReconcilePendingOrder(fctx, o.ID)
				if err != nil {
					return err
				}
				if res.Settled {
					settled++
				}
				return nil
			})
			if err != nil {
				logger.Warn("payment reconciler: reconcile failed",
					zap.String("orderId", o.ID),
					zap.String("tenantId", o.TenantID),
					zap.Error(err),
				)
			}
		}
		if settled > 0 {
			// Every one of these is a customer who paid and would otherwise have
			// been left without a pickup QR.
			logger.Info("settled orders missed by webhooks",
				zap.Int("count", settled),
				zap.Int("candidates", len(candidates)),
			)
		}
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(rc.Interval)
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
			logger.Info("payment reconciler started",
				zap.Duration("interval", rc.Interval),
				zap.Duration("minAge", rc.MinAge),
				zap.Duration("maxAge", rc.MaxAge),
			)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}
