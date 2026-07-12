package repository

import (
	"context"

	"backend/internal/inventory"

	"go.uber.org/zap"
)

// InventoryNotifier implements inventory.Publisher by emitting a Postgres NOTIFY
// on the request tx, so an update is delivered on commit — a rolled-back stock
// change never emits a phantom update.
type InventoryNotifier struct {
	logger *zap.Logger
}

func NewInventoryNotifier(logger *zap.Logger) *InventoryNotifier {
	return &InventoryNotifier{logger: logger}
}

func (n *InventoryNotifier) Publish(ctx context.Context, u inventory.Update) {
	if u.TenantID == "" {
		return
	}
	tx, ok := RequestTxFromContext(ctx)
	if !ok {
		n.logger.Warn("inventory notify: no request tx in context; update dropped",
			zap.String("productId", u.ProductID))
		return
	}
	payload, err := inventory.MarshalUpdate(u)
	if err != nil {
		n.logger.Warn("inventory notify: marshal failed", zap.Error(err))
		return
	}
	if _, err := tx.ExecContext(ctx, "SELECT pg_notify($1, $2)", inventory.NotifyChannel, payload); err != nil {
		n.logger.Warn("inventory notify: pg_notify failed", zap.Error(err))
	}
}
