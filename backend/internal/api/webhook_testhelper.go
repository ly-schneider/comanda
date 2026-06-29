package api

import (
	"backend/internal/database"
	"backend/internal/generated/ent"
	"backend/internal/payment"
	"backend/internal/service"

	"go.uber.org/zap"
)

// NewWebhookHandlers lets the cross-tenant webhook integration tests drive the
// real HandlePayrexxWebhook flow without standing up the full FX graph.
func NewWebhookHandlers(
	payments service.PaymentService,
	refunds service.RefundService,
	privileged *ent.Client,
	appDB *database.AppClient,
	providers *payment.Factory,
	logger *zap.Logger,
) *Handlers {
	return &Handlers{
		payments:   payments,
		refunds:    refunds,
		privileged: privileged,
		appDB:      appDB,
		providers:  providers,
		logger:     logger,
	}
}
