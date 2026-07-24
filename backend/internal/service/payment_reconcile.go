package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"backend/internal/generated/ent/order"
	"backend/internal/payrexx"
	"backend/internal/trace"

	"go.uber.org/zap"
)

const (
	reconcilePullTimeout  = 5 * time.Second
	reconcilePullInterval = 3 * time.Second
	// A request-path pull holds a pooled DB connection across an external HTTP
	// call, and connection exhaustion — not CPU — is this deployment's ceiling.
	reconcileMaxInFlight = 8
)

type ReconcileResult struct {
	// Checked is false when no pull happened: already settled, no gateway,
	// provider unconfigured, or the request-path guard declined.
	Checked bool
	// Settled is true only when this call moved the order from pending to paid.
	Settled bool
	Paid    bool
	Failed  bool
}

type reconcileGuard struct {
	mu    sync.Mutex
	last  map[string]time.Time
	slots chan struct{}
}

func newReconcileGuard() *reconcileGuard {
	return &reconcileGuard{
		last:  make(map[string]time.Time),
		slots: make(chan struct{}, reconcileMaxInFlight),
	}
}

// tryBegin never blocks. The slot is taken before the cooldown is stamped, so a
// pull turned away for capacity doesn't mute the order's next few polls.
func (g *reconcileGuard) tryBegin(key string, now time.Time) (func(), bool) {
	select {
	case g.slots <- struct{}{}:
	default:
		return nil, false
	}
	release := func() { <-g.slots }

	g.mu.Lock()
	defer g.mu.Unlock()
	for k, t := range g.last {
		if now.Sub(t) > 10*reconcilePullInterval {
			delete(g.last, k)
		}
	}
	if t, ok := g.last[key]; ok && now.Sub(t) < reconcilePullInterval {
		release()
		return nil, false
	}
	g.last[key] = now
	return release, true
}

// TryReconcilePendingOrder is the request-path entry point. A declined pull is
// not an error: answer from the database and leave it to the background pass.
func (s *paymentService) TryReconcilePendingOrder(ctx context.Context, orderID string) (ReconcileResult, error) {
	release, ok := s.reconcile.tryBegin(orderID, time.Now())
	if !ok {
		return ReconcileResult{}, nil
	}
	defer release()
	return s.ReconcilePendingOrder(ctx, orderID)
}

// ReconcilePendingOrder settles a pending order from the provider's own view of
// its checkout, covering webhooks that never arrive. Idempotent, and a no-op for
// any order that is already settled or was never sent to a provider.
func (s *paymentService) ReconcilePendingOrder(ctx context.Context, orderID string) (ReconcileResult, error) {
	ctx, finish := trace.StartSpan(ctx, "service", "payment.reconcile_pending")
	defer finish()
	trace.Data(ctx, "reconcile.order_id", orderID)

	ord, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		trace.Err(ctx, err)
		return ReconcileResult{}, err
	}
	if ord.Status == order.StatusPaid {
		return ReconcileResult{Paid: true}, nil
	}
	if ord.Status != order.StatusPending || ord.PayrexxGatewayID == nil || s.providers == nil {
		return ReconcileResult{}, nil
	}

	// No usable credentials is "can't check", not a failure — an error here would
	// block the sweeper that depends on this verdict and leak the hold forever.
	provider, err := s.providers.For(ctx)
	if err != nil {
		trace.Data(ctx, "reconcile.skipped", "provider_unavailable")
		return ReconcileResult{}, nil
	}

	pullCtx, cancel := context.WithTimeout(ctx, reconcilePullTimeout)
	defer cancel()

	gatewayID := *ord.PayrexxGatewayID
	checkout, err := provider.GetCheckout(pullCtx, gatewayID)
	if err != nil {
		// A gateway the provider has expunged is a definitive "no payment here";
		// as an error it would defer the sweeper's release forever.
		if errors.Is(err, payrexx.ErrNotFound) {
			return ReconcileResult{Checked: true, Failed: true}, nil
		}
		trace.Err(ctx, err)
		return ReconcileResult{}, err
	}

	out := ReconcileResult{Checked: true, Failed: checkout.Settlement.Failed}
	trace.Data(ctx, "reconcile.provider_status", checkout.Status)
	if !checkout.Settlement.Paid {
		return out, nil
	}

	var contactEmail *string
	if checkout.Settlement.ContactEmail != "" {
		contactEmail = &checkout.Settlement.ContactEmail
	}
	if err := s.MarkOrderPaidByPayrexx(ctx, orderID, gatewayID, checkout.Settlement.TransactionID, contactEmail); err != nil {
		trace.Err(ctx, err)
		return out, err
	}

	if checkout.Settlement.TransactionID == 0 {
		// Refunding this order needs the Payrexx dashboard until a webhook backfills.
		s.logger.Warn("reconciled payment without transaction id",
			zap.String("orderId", orderID),
			zap.Int("gatewayId", gatewayID),
		)
	}
	s.logger.Info("settled order by provider reconcile",
		zap.String("orderId", orderID),
		zap.Int("gatewayId", gatewayID),
		zap.String("providerStatus", checkout.Status),
	)

	out.Settled = true
	out.Paid = true
	return out, nil
}
