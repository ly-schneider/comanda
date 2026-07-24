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

// StartPaymentReconciler settles orders whose success webhook never arrived —
// delayed, dropped, or delivered while a pod was rolling — so a customer who has
// paid gets their pickup QR without ever coming back from the TWINT app.
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
			// Don't look like full coverage when the window held more than a batch.
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
