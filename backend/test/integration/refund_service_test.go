package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/refund"
	"backend/internal/payment"
	"backend/internal/qrsign"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubRefundCall struct {
	txnID  int
	amount int
}

// stubRefundProvider records Refund calls and otherwise no-ops the rest of the
// payment.Provider surface. The mutex keeps it race-safe under the concurrent
// refund test.
type stubRefundProvider struct {
	mu    sync.Mutex
	calls []stubRefundCall
	err   error
}

func (p *stubRefundProvider) Refund(_ context.Context, txnID, amount int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, stubRefundCall{txnID: txnID, amount: amount})
	return p.err
}

func (p *stubRefundProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func (p *stubRefundProvider) CreateCheckout(context.Context, payment.CheckoutRequest) (*payment.CheckoutResult, error) {
	return nil, nil
}
func (p *stubRefundProvider) GetCheckout(context.Context, int) (*payment.CheckoutResult, error) {
	return nil, nil
}
func (p *stubRefundProvider) VerifyWebhook([]byte, string) bool { return true }
func (p *stubRefundProvider) ParseWebhook([]byte) (*payment.WebhookResult, error) {
	return nil, nil
}

type stubResolver struct{ p payment.Provider }

func (r stubResolver) For(context.Context) (payment.Provider, error) { return r.p, nil }

func newRefundSvc(repos *Repositories, prov *stubRefundProvider) service.RefundService {
	return service.NewRefundService(
		repos.Order,
		repos.Refund,
		repos.Inventory,
		stubResolver{p: prov},
		NewQRKeySvc(repos),
		nil,
		zap.NewNop(),
	)
}

// createPaidOrder builds a paid order with one simple-product line (qty), an
// initial signed pickup token, reserved inventory, and optionally a Payrexx
// transaction id (TWINT) — returning the product id and opening stock.
func createPaidOrder(t *testing.T, ctx context.Context, repos *Repositories, paymentSvc service.PaymentService, qty int, payrexxTxnID *int) (*orderFixture, string) {
	t.Helper()
	fixtures := NewFixtures(repos)
	category := fixtures.CreateCategory("Drinks-"+randSuffix(), 1, true)
	prod := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	fixtures.AddInventory(prod.ID, 100, inventoryledger.ReasonOpeningBalance)

	prep, err := paymentSvc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
		Items: []service.CheckoutItemInput{{ProductID: prod.ID, Quantity: qty}},
	}, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, prep.QRPayload)

	ord, err := repos.Order.GetByID(ctx, prep.OrderID)
	require.NoError(t, err)
	_, err = repos.Order.Update(ctx, ord.ID, ord.TotalCents, order.StatusPaid, ord.Origin, ord.CustomerID, ord.ContactEmail, ord.PaymentAttemptID, ord.PayrexxGatewayID, payrexxTxnID)
	require.NoError(t, err)

	return &orderFixture{orderID: ord.ID, initialToken: prep.QRPayload, total: ord.TotalCents}, prod.ID
}

type orderFixture struct {
	orderID      string
	initialToken string
	total        int64
}

func TestRefundService_LineRefundTwint(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)
	keys := NewQRKeySvc(repos)

	txnID := 778899
	of, prodID := createPaidOrder(t, ctx, repos, paymentSvc, 3, &txnID)

	lineID := firstLineID(t, ctx, repos, of.orderID)
	stockBefore := currentStock(t, ctx, repos, prodID)

	ref, err := svc.RefundLines(ctx, service.RefundLinesInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: lineID, Quantity: 1}},
		Actor:   service.RefundActor{Type: refund.ActorTypeOperator, ID: "op_1"},
	})
	require.NoError(t, err)

	// Derived amount = unit price (350) * 1, routed to TWINT via the provider.
	require.Equal(t, int64(350), ref.AmountCents)
	require.Equal(t, refund.MethodTwint, ref.Method)
	require.Equal(t, refund.StatusSucceeded, ref.Status)
	require.Len(t, prov.calls, 1)
	require.Equal(t, stubRefundCall{txnID: txnID, amount: 350}, prov.calls[0])

	// Partial refund -> partially_refunded.
	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusPartiallyRefunded, ord.Status)

	// Inventory restored for the refunded unit.
	require.Equal(t, stockBefore+1, currentStock(t, ctx, repos, prodID))

	// QR re-issued: old superseded, new verifies, remaining qty = 2.
	require.NotNil(t, ord.QrPayload)
	require.NotEqual(t, of.initialToken, *ord.QrPayload, "refund must supersede the old pickup token")
	_, pub, err := keys.EnsureTenantKey(ctx)
	require.NoError(t, err)
	payload, err := qrsign.Verify(pub, *ord.QrPayload)
	require.NoError(t, err)
	require.Len(t, payload.Lines, 1)
	require.Equal(t, prodID, payload.Lines[0].ProductID)
	require.Equal(t, 2, payload.Lines[0].Quantity)
}

func TestRefundService_OverRefundRejected(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 5
	of, _ := createPaidOrder(t, ctx, repos, paymentSvc, 2, &txnID)
	lineID := firstLineID(t, ctx, repos, of.orderID)

	_, err := svc.RefundLines(ctx, service.RefundLinesInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: lineID, Quantity: 3}}, // only 2 bought
		Actor:   service.RefundActor{Type: refund.ActorTypeOperator, ID: "op_1"},
	})
	require.Error(t, err)
	require.Empty(t, prov.calls, "a rejected refund must not call the provider")

	refs, err := repos.Refund.ListByOrder(ctx, of.orderID)
	require.NoError(t, err)
	require.Empty(t, refs, "a rejected refund must not persist a refund row")
}

func TestRefundService_CardCashOut(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	// No Payrexx transaction id -> card/cash order, refund is a cash-out.
	of, _ := createPaidOrder(t, ctx, repos, paymentSvc, 1, nil)
	lineID := firstLineID(t, ctx, repos, of.orderID)

	ref, err := svc.RefundLines(ctx, service.RefundLinesInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: lineID, Quantity: 1}},
		Actor:   service.RefundActor{Type: refund.ActorTypeAdmin, ID: "admin_1"},
	})
	require.NoError(t, err)
	require.Equal(t, refund.MethodCash, ref.Method)
	require.Empty(t, prov.calls, "a cash-out refund must not call the payment provider")

	// Full single-line refund -> refunded.
	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusRefunded, ord.Status)
}

func TestRefundService_RedeemedLineMoneyOnly(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)
	keys := NewQRKeySvc(repos)

	of, _ := createPaidOrder(t, ctx, repos, paymentSvc, 1, nil)
	lineID := firstLineID(t, ctx, repos, of.orderID)

	// The single line is picked up (redeemed) before the refund.
	_, err := repos.OrderRedemption.Create(ctx, lineID, time.Now())
	require.NoError(t, err)

	ref, err := svc.RefundLines(ctx, service.RefundLinesInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: lineID, Quantity: 1}},
		Actor:   service.RefundActor{Type: refund.ActorTypeAdmin, ID: "admin_1"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(350), ref.AmountCents)

	// The re-issued pickup token carries no remaining items: the only line was
	// already handed over, so the refund is money-only.
	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.NotNil(t, ord.QrPayload)
	_, pub, err := keys.EnsureTenantKey(ctx)
	require.NoError(t, err)
	payload, err := qrsign.Verify(pub, *ord.QrPayload)
	require.NoError(t, err)
	require.Empty(t, payload.Lines, "a redeemed line must not appear in the re-issued pickup set")
}

func TestRefundService_CancelClearsQR(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 4242
	of, prodID := createPaidOrder(t, ctx, repos, paymentSvc, 2, &txnID)
	stockBefore := currentStock(t, ctx, repos, prodID)

	ref, err := svc.CancelOrder(ctx, service.CancelOrderInput{
		OrderID: of.orderID,
		Actor:   service.RefundActor{Type: refund.ActorTypeAdmin, ID: "admin_1"},
	})
	require.NoError(t, err)
	require.Equal(t, of.total, ref.AmountCents)
	require.Len(t, prov.calls, 1)
	require.Equal(t, of.total, int64(prov.calls[0].amount))

	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusCancelled, ord.Status)
	require.Nil(t, ord.QrPayload, "a full cancellation clears the pickup token")

	// All remaining units restored.
	require.Equal(t, stockBefore+2, currentStock(t, ctx, repos, prodID))
}

func TestRefundService_ArbitraryRequiresReasonAndCaps(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 9
	of, _ := createPaidOrder(t, ctx, repos, paymentSvc, 2, &txnID) // total 700

	_, err := svc.RefundArbitrary(ctx, service.RefundArbitraryInput{
		OrderID:     of.orderID,
		AmountCents: 100,
		Reason:      "   ",
		Actor:       service.RefundActor{Type: refund.ActorTypeAdmin, ID: "admin_1"},
	})
	require.Error(t, err, "arbitrary refund must require a reason")

	// Over-cap request is capped at the refundable balance (700).
	ref, err := svc.RefundArbitrary(ctx, service.RefundArbitraryInput{
		OrderID:     of.orderID,
		AmountCents: 5000,
		Reason:      "goodwill",
		Actor:       service.RefundActor{Type: refund.ActorTypeAdmin, ID: "admin_1"},
	})
	require.NoError(t, err)
	require.Equal(t, of.total, ref.AmountCents)
	require.Equal(t, refund.KindArbitrary, ref.Kind)

	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusRefunded, ord.Status)
}

func firstLineID(t *testing.T, ctx context.Context, repos *Repositories, orderID string) string {
	t.Helper()
	lines, err := repos.OrderLine.GetByOrderID(ctx, orderID)
	require.NoError(t, err)
	require.NotEmpty(t, lines)
	for _, l := range lines {
		if l.ParentLineID == nil {
			return l.ID
		}
	}
	return lines[0].ID
}

func currentStock(t *testing.T, ctx context.Context, repos *Repositories, productID string) int {
	t.Helper()
	stock, err := repos.Inventory.GetCurrentStock(ctx, productID)
	require.NoError(t, err)
	return stock
}
