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

// The webhook is one delivery attempt at our API; when it is late, dropped, or
// lands mid-rollout the customer has paid and has no pickup QR. Reconciling
// asks the provider directly, so confirmation never depends on a push arriving.
const (
	// reconcilePullTimeout bounds how long a provider call may pin the caller.
	// On the request path that caller holds the request's DB transaction, so
	// this stays well under any client-side poll interval.
	reconcilePullTimeout = 5 * time.Second
	// reconcilePullInterval is the minimum spacing between provider pulls for
	// one order on the request path, where a waiting customer polls every
	// couple of seconds.
	reconcilePullInterval = 3 * time.Second
	// reconcileMaxInFlight caps concurrent request-path pulls. Each one holds a
	// pooled DB connection across an external HTTP call, and connection
	// exhaustion — not CPU — is this deployment's known ceiling.
	reconcileMaxInFlight = 8
)

// ReconcileResult reports what a provider pull established about an order.
type ReconcileResult struct {
	// Checked is false when no pull happened (order already settled, no gateway,
	// provider unconfigured, or the request-path guard declined).
	Checked bool
	// Settled is true only when this call moved the order from pending to paid.
	Settled bool
	// Paid reflects the order's state after the call.
	Paid bool
	// Failed means the provider reports a terminal failure — the money will not
	// arrive and the hold can be released.
	Failed bool
}

// reconcileGuard keeps request-path pulls from becoming a load amplifier: a
// per-order cooldown (many customers polling one order) plus a global
// concurrency cap (many customers polling many orders).
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

// tryBegin reserves the right to pull for one order, or reports false when the
// caller should just answer from the database. Never blocks.
//
// The slot is taken before the cooldown is stamped so that a pull turned away
// for lack of capacity doesn't start a cooldown it never used — otherwise a busy
// moment would silently mute an order's next few polls.
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

// TryReconcilePendingOrder is the request-path entry point: it reconciles when
// the shared pull budget allows and otherwise reports Checked=false, leaving the
// order for the background reconciler. A caller must not treat a declined pull
// as an error — answering from the database is the correct fallback.
func (s *paymentService) TryReconcilePendingOrder(ctx context.Context, orderID string) (ReconcileResult, error) {
	release, ok := s.reconcile.tryBegin(orderID, time.Now())
	if !ok {
		return ReconcileResult{}, nil
	}
	defer release()
	return s.ReconcilePendingOrder(ctx, orderID)
}

// ReconcilePendingOrder asks the payment provider what actually happened to a
// pending order's checkout and settles it when the money is in. It is
// idempotent and safe to run against any order: anything already settled, never
// sent to a provider, or belonging to a tenant without a configured provider is
// a no-op.
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

	// No usable credentials (never configured, or disconnected since this order
	// was placed) is "can't check", not a failure. Returning an error here would
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
		// A gateway the provider no longer knows about (expired and expunged) is
		// a definitive "no payment here". Reporting it as an error would make the
		// sweeper defer release forever and strand the reserved stock.
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
		// Settled without a transaction id: the customer is served, but a later
		// refund for this order has to be issued from the Payrexx dashboard
		// unless a webhook arrives to backfill it.
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
