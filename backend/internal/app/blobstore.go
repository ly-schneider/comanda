package app

import (
	"context"

	"backend/internal/blobstore"
	"backend/internal/config"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

func NewBlobStore(lc fx.Lifecycle, cfg config.Config, logger *zap.Logger) *blobstore.Client {
	b := cfg.BlobStorage
	if b.Endpoint == "" || b.AccessKeyID == "" || b.SecretAccessKey == "" || b.PublicBaseURL == "" {
		logger.Warn("R2 storage not configured, image uploads disabled")
		return nil
	}

	client, err := blobstore.NewClient(
		b.Endpoint,
		b.AccessKeyID,
		b.SecretAccessKey,
		b.Bucket,
		b.PublicBaseURL,
	)
	if err != nil {
		logger.Error("failed to create R2 storage client", zap.Error(err))
		return nil
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := client.EnsureBucket(ctx); err != nil {
				logger.Warn("failed to reach R2 bucket", zap.Error(err))
			}
			return nil
		},
	})

	return client
}
