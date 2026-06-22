package app

import (
	"context"
	"sync"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/k8singress"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// NewIngressReconciler builds the custom-domain ingress reconciler. When the
// reconciler is disabled (the default) this returns a no-op, mirroring how the
// blob store degrades when unconfigured.
func NewIngressReconciler(cfg config.Config, priv *database.PrivilegedClient, logger *zap.Logger) (k8singress.Reconciler, error) {
	return k8singress.New(priv.Client, cfg.Ingress, logger)
}

// StartIngressReconciler runs the reconciler on a ticker: once promptly at start
// and then on each interval. Each pass is bounded by its own timeout and errors
// are logged, never fatal — a transient cluster hiccup must not crash the API.
func StartIngressReconciler(
	lc fx.Lifecycle,
	cfg config.Config,
	reconciler k8singress.Reconciler,
	logger *zap.Logger,
) {
	if !cfg.Ingress.Enabled {
		return
	}

	interval := cfg.Ingress.ReconcileEvery
	if interval <= 0 {
		interval = 60 * time.Second
	}

	passTimeout := cfg.Ingress.ReconcileTimeout
	if passTimeout <= 0 {
		passTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	runPass := func() {
		passCtx, passCancel := context.WithTimeout(ctx, passTimeout)
		defer passCancel()
		if err := reconciler.Reconcile(passCtx); err != nil {
			logger.Warn("ingress reconcile pass failed", zap.Error(err))
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
			logger.Info("ingress reconciler started",
				zap.String("namespace", cfg.Ingress.Namespace),
				zap.Duration("interval", interval))
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}
