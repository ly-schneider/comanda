package app

import (
	"context"
	"sync"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/inventory"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// StartInventoryListener runs one Postgres LISTEN connection per pod and fans
// received updates out to this pod's local Hub — one dedicated DB connection per
// pod (not per client) is what makes inventory correct across replicas.
func StartInventoryListener(
	lc fx.Lifecycle,
	cfg config.Config,
	hub *inventory.Hub,
	logger *zap.Logger,
) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runInventoryListener(ctx, cfg, hub, logger)
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}

func runInventoryListener(ctx context.Context, cfg config.Config, hub *inventory.Hub, logger *zap.Logger) {
	const backoff = 2 * time.Second
	for ctx.Err() == nil {
		conn, err := database.NewListenConn(ctx, cfg)
		if err != nil {
			logger.Warn("inventory listener: connect failed", zap.Error(err))
			listenerSleep(ctx, backoff)
			continue
		}
		if _, err := conn.Exec(ctx, "LISTEN "+inventory.NotifyChannel); err != nil {
			logger.Warn("inventory listener: LISTEN failed", zap.Error(err))
			_ = conn.Close(context.Background())
			listenerSleep(ctx, backoff)
			continue
		}
		logger.Info("inventory listener connected")

		for ctx.Err() == nil {
			n, err := conn.WaitForNotification(ctx)
			if err != nil {
				if ctx.Err() == nil {
					logger.Warn("inventory listener: wait failed, reconnecting", zap.Error(err))
				}
				break
			}
			update, err := inventory.ParseUpdate(n.Payload)
			if err != nil {
				logger.Warn("inventory listener: bad payload", zap.Error(err))
				continue
			}
			hub.Publish(update)
		}
		_ = conn.Close(context.Background())
	}
}

func listenerSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
