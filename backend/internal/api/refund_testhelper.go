package api

import (
	"backend/internal/service"

	"go.uber.org/zap"
)

// NewRefundHandlers lets the refund HTTP integration tests drive the generated
// ServerInterface refund methods without standing up the full FX graph. The
// refund handlers only reach h.refunds (+ the pure mappers), so nothing else
// needs wiring.
func NewRefundHandlers(refunds service.RefundService, logger *zap.Logger) *Handlers {
	return &Handlers{
		refunds: refunds,
		logger:  logger,
	}
}
