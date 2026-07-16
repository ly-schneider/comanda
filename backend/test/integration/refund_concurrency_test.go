package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/refund"
	"backend/internal/payment"
	pgRepo "backend/internal/repository"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestRefundService_ConcurrentRefundSerialized proves the FOR UPDATE lock in
// loadRefundableOrder serializes two concurrent full refunds of the same order:
// without it both would read a zero refunded-sum, both pass the over-refund
// guard, and both call provider.Refund — a double payment. With it, the second
// refund blocks until the first commits, then sees the fully-refunded order and
// is rejected. Exactly one refund (and one provider call) gets through.
//
// Uses RefundArbitrary so the flow has no QR/inventory dependencies; the lock is
// taken in loadRefundableOrder, shared by every refund entry point.
func TestRefundService_ConcurrentRefundSerialized(t *testing.T) {
	env := newRLSEnv(t)
	tenant := env.seedTenant(t, "refund-conc-"+randSuffix())

	orderID := nanoid.New()
	env.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, payrexx_transaction_id, tenant_id)
		 VALUES ($1, 350, 'paid', 'pos', 111, $2)`,
		orderID, tenant,
	)

	prov := &stubRefundProvider{}

	// Each goroutine runs in its own request tx; the deferred cleanup commits,
	// releasing the row lock so the blocked goroutine can proceed.
	run := func() error {
		ctx, client, cleanup := env.appRequestTx(t, tenant)
		defer cleanup()
		svc := service.NewRefundService(
			pgRepo.NewOrderRepository(client),
			pgRepo.NewRefundRepository(client),
			pgRepo.NewInventoryLedgerRepository(client),
			stubResolver{p: prov},
			nil, // qrKeys: arbitrary refunds do not re-issue the pickup token
			nil,
			zap.NewNop(),
		)
		_, err := svc.RefundArbitrary(ctx, service.RefundArbitraryInput{
			OrderID:     orderID,
			AmountCents: 350,
			Reason:      "concurrent",
			Actor:       service.RefundActor{Type: refund.ActorTypeAdmin, ID: "admin_1"},
		})
		return err
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = run()
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, e := range errs {
		if e == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent refund may succeed")
	require.Equal(t, 1, prov.callCount(), "provider.Refund must run exactly once — no double payment")

	ctx, client, cleanup := env.appRequestTx(t, tenant)
	defer cleanup()
	sum, err := pgRepo.NewRefundRepository(client).SumSucceededByOrder(ctx, orderID)
	require.NoError(t, err)
	require.Equal(t, int64(350), sum, "total refunded must equal the order total, not double it")

	ord, err := pgRepo.NewOrderRepository(client).GetByID(ctx, orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusRefunded, ord.Status)
}

// blockingRefundProvider holds provider.Refund open until released, so a test can run the
// webhook reconcile while a synchronous refund still holds the order lock.
type blockingRefundProvider struct {
	*stubRefundProvider
	inFlight chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (p *blockingRefundProvider) Refund(ctx context.Context, txnID, amount int) error {
	p.once.Do(func() { close(p.inFlight) })
	<-p.release
	return p.stubRefundProvider.Refund(ctx, txnID, amount)
}

// TestRefundService_WebhookReconcileSerializedWithSyncRefund proves the FOR UPDATE lock
// in ReconcileFromWebhook serializes it behind an in-flight synchronous refund, so a
// racing webhook reconcile dedupes to one refund row instead of recording a duplicate.
func TestRefundService_WebhookReconcileSerializedWithSyncRefund(t *testing.T) {
	env := newRLSEnv(t)
	tenant := env.seedTenant(t, "reconcile-race-"+randSuffix())

	orderID := nanoid.New()
	const txnID = 222
	env.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, payrexx_transaction_id, tenant_id)
		 VALUES ($1, 350, 'paid', 'pos', $2, $3)`,
		orderID, txnID, tenant,
	)

	prov := &blockingRefundProvider{
		stubRefundProvider: &stubRefundProvider{},
		inFlight:           make(chan struct{}),
		release:            make(chan struct{}),
	}

	var wg sync.WaitGroup
	var syncErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, client, cleanup := env.appRequestTx(t, tenant)
		defer cleanup()
		svc := service.NewRefundService(
			pgRepo.NewOrderRepository(client),
			pgRepo.NewRefundRepository(client),
			pgRepo.NewInventoryLedgerRepository(client),
			stubResolver{p: prov},
			nil, nil, zap.NewNop(),
		)
		_, syncErr = svc.RefundArbitrary(ctx, service.RefundArbitraryInput{
			OrderID:     orderID,
			AmountCents: 200,
			Reason:      "sync",
			Actor:       service.RefundActor{Type: refund.ActorTypeAdmin, ID: "admin_sync"},
		})
	}()

	// Sync refund is now inside provider.Refund, holding the order lock uncommitted.
	<-prov.inFlight

	var recErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, client, cleanup := env.appRequestTx(t, tenant)
		defer cleanup()
		svc := service.NewRefundService(
			pgRepo.NewOrderRepository(client),
			pgRepo.NewRefundRepository(client),
			pgRepo.NewInventoryLedgerRepository(client),
			stubResolver{p: prov},
			nil, nil, zap.NewNop(),
		)
		recErr = svc.ReconcileFromWebhook(ctx, &payment.WebhookResult{
			ReferenceID:         orderID,
			IsRefund:            true,
			RefundedAmountCents: 200,
			TransactionID:       txnID,
		})
	}()

	// Let the reconcile reach the lock (without the fix: read + insert its duplicate)
	// before the sync refund commits.
	time.Sleep(400 * time.Millisecond)
	close(prov.release)

	wg.Wait()
	require.NoError(t, syncErr)
	require.NoError(t, recErr)

	require.Equal(t, 1, prov.callCount(), "the sync refund calls the provider exactly once")
	require.Equal(t, 1, env.countRefunds(t, orderID),
		"exactly one refund row — the webhook reconcile must dedupe against the sync refund")

	ctx, client, cleanup := env.appRequestTx(t, tenant)
	defer cleanup()
	sum, err := pgRepo.NewRefundRepository(client).SumSucceededByOrder(ctx, orderID)
	require.NoError(t, err)
	require.Equal(t, int64(200), sum, "recorded refund must match the sync refund, not double it")
}
