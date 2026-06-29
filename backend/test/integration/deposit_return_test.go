package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/qrsign"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

var (
	opActor    = service.RefundActor{Type: "operator", ID: "op_1"}
	adminActor = service.RefundActor{Type: "admin", ID: "admin_1"}
)

// createPaidDepositOrder builds a paid order of `qty` units of a single product
// carrying `depositCents` per unit, returning the order fixture plus the product,
// product-line, and deposit-line ids.
func createPaidDepositOrder(t *testing.T, ctx context.Context, repos *Repositories, paymentSvc service.PaymentService, qty int, depositCents int64, payrexxTxnID *int) (of *orderFixture, prodID, productLineID, depositLineID string) {
	t.Helper()
	fixtures := NewFixtures(repos)
	category := fixtures.CreateCategory("Mugs-"+randSuffix(), 1, true)
	prod := fixtures.CreateProductWithDeposit("Mug", category.ID, 350, depositCents)
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

	lines, err := repos.OrderLine.GetByOrderID(ctx, ord.ID)
	require.NoError(t, err)
	for _, l := range lines {
		switch l.LineType {
		case orderline.LineTypeDeposit:
			depositLineID = l.ID
		case orderline.LineTypeSimple:
			productLineID = l.ID
		}
	}
	require.NotEmpty(t, productLineID, "product line present")
	require.NotEmpty(t, depositLineID, "deposit line present")

	return &orderFixture{orderID: ord.ID, initialToken: prep.QRPayload, total: ord.TotalCents}, prod.ID, productLineID, depositLineID
}

func depositOutstanding(t *testing.T, ctx context.Context, svc service.RefundService, orderID, depositLineID string) int {
	t.Helper()
	view, err := svc.GetRefundView(ctx, orderID)
	require.NoError(t, err)
	for _, d := range view.Deposits {
		if d.Line.ID == depositLineID {
			return d.OutstandingQty
		}
	}
	t.Fatalf("deposit line %s not present in refund view", depositLineID)
	return -1
}

// Scenario: scan order QR and refund deposit to TWINT (online order).
func TestDepositReturn_TwintScanQR(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 4711
	of, _, _, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 1, 200, &txnID)

	token := of.initialToken
	ref, err := svc.ReturnDeposit(ctx, service.DepositReturnInput{
		OrderID: of.orderID,
		Token:   &token,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 1}},
		Actor:   opActor,
	})
	require.NoError(t, err)
	require.Equal(t, int64(200), ref.AmountCents)
	require.Equal(t, "twint", string(ref.Method))
	require.Equal(t, "succeeded", string(ref.Status))
	require.Len(t, prov.calls, 1)
	require.Equal(t, stubRefundCall{txnID: txnID, amount: 200}, prov.calls[0])

	require.Equal(t, 0, depositOutstanding(t, ctx, svc, of.orderID, depositLineID))

	// The pickup QR is untouched (deposits aren't redeemable items); the order has
	// its product (350) still owed, so it is only partially refunded.
	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.NotNil(t, ord.QrPayload)
	require.Equal(t, of.initialToken, *ord.QrPayload, "a deposit return must not re-issue the pickup token")
	require.Equal(t, order.StatusPartiallyRefunded, ord.Status)
}

// Scenario: per-unit partial return.
func TestDepositReturn_PerUnitPartial(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 5
	of, _, _, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 3, 200, &txnID)

	ref, err := svc.ReturnDeposit(ctx, service.DepositReturnInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 2}},
		Actor:   opActor,
	})
	require.NoError(t, err)
	require.Equal(t, int64(400), ref.AmountCents)
	require.Equal(t, 1, depositOutstanding(t, ctx, svc, of.orderID, depositLineID))
}

// Scenario: cash-paid order refunds the deposit as cash.
func TestDepositReturn_CashOut(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	of, _, _, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 1, 200, nil)

	ref, err := svc.ReturnDeposit(ctx, service.DepositReturnInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 1}},
		Actor:   adminActor,
	})
	require.NoError(t, err)
	require.Equal(t, "cash", string(ref.Method))
	require.Empty(t, prov.calls, "a cash-out deposit return must not call the payment provider")
	require.Equal(t, 0, depositOutstanding(t, ctx, svc, of.orderID, depositLineID))
}

// Scenario: refunding a product line releases its linked deposit.
func TestDepositReturn_ProductLineReleasesDeposit(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 99
	of, _, productLineID, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 1, 200, &txnID)

	ref, err := svc.RefundLines(ctx, service.RefundLinesInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: productLineID, Quantity: 1}},
		Actor:   opActor,
	})
	require.NoError(t, err)
	// 350 product + 200 linked deposit, refunded together against the transaction.
	require.Equal(t, int64(550), ref.AmountCents)
	require.Len(t, prov.calls, 1)
	require.Equal(t, 550, prov.calls[0].amount)

	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusRefunded, ord.Status)
	require.Equal(t, 0, depositOutstanding(t, ctx, svc, of.orderID, depositLineID))

	// Nothing left to return.
	_, err = svc.ReturnDeposit(ctx, service.DepositReturnInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 1}},
		Actor:   opActor,
	})
	require.Error(t, err)
}

// Scenario: full refund / cancel settles outstanding deposits.
func TestDepositReturn_CancelSettlesDeposits(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 7
	of, prodID, _, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 2, 200, &txnID)
	stockBefore := currentStock(t, ctx, repos, prodID)

	ref, err := svc.CancelOrder(ctx, service.CancelOrderInput{OrderID: of.orderID, Actor: adminActor})
	require.NoError(t, err)
	// 2*(350+200) = 1100 — products and outstanding deposits both refunded.
	require.Equal(t, int64(1100), ref.AmountCents)
	require.Len(t, prov.calls, 1)
	require.Equal(t, 1100, prov.calls[0].amount)

	ord, err := repos.Order.GetByID(ctx, of.orderID)
	require.NoError(t, err)
	require.Equal(t, order.StatusCancelled, ord.Status)

	// Only the two product units are restored — deposit lines must not double-restore.
	require.Equal(t, stockBefore+2, currentStock(t, ctx, repos, prodID))

	// The deposit is settled, so a later return scan finds nothing outstanding.
	require.Equal(t, 0, depositOutstanding(t, ctx, svc, of.orderID, depositLineID))
}

// Scenario: over-/double-return is prevented.
func TestDepositReturn_OverAndDoubleReturnRejected(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 22
	of, _, _, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 3, 200, &txnID)

	// Asking for more than outstanding in one call is rejected with no side effects.
	_, err := svc.ReturnDeposit(ctx, service.DepositReturnInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 4}},
		Actor:   opActor,
	})
	require.Error(t, err)
	require.Empty(t, prov.calls, "a rejected return must not call the provider")
	refs, err := repos.Refund.ListByOrder(ctx, of.orderID)
	require.NoError(t, err)
	require.Empty(t, refs, "a rejected return must not persist a refund row")

	// Return all three, then a further return is refused.
	_, err = svc.ReturnDeposit(ctx, service.DepositReturnInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 3}},
		Actor:   opActor,
	})
	require.NoError(t, err)
	require.Len(t, prov.calls, 1)

	_, err = svc.ReturnDeposit(ctx, service.DepositReturnInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 1}},
		Actor:   opActor,
	})
	require.Error(t, err, "a fully-returned deposit cannot be returned again")
	require.Len(t, prov.calls, 1, "the rejected double-return must not call the provider")
}

// Scenario: invalid or wrong-tenant QR is rejected.
func TestDepositReturn_InvalidTokenRejected(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)
	keys := NewQRKeySvc(repos)

	txnID := 31
	of, _, _, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 1, 200, &txnID)
	priv, _, err := keys.EnsureTenantKey(ctx)
	require.NoError(t, err)

	garbage := "not-a-real-token"

	// Valid tenant signature, but for a different order id.
	mismatched, err := qrsign.Sign(priv, qrsign.Payload{Version: qrsign.Version, TenantID: DefaultTestTenantID, OrderID: "other_order1", IssuedAt: 1})
	require.NoError(t, err)

	// A token signed by a foreign key fails signature verification (wrong tenant).
	_, foreignPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	foreign, err := qrsign.Sign(foreignPriv, qrsign.Payload{Version: qrsign.Version, TenantID: DefaultTestTenantID, OrderID: of.orderID, IssuedAt: 1})
	require.NoError(t, err)

	for name, tok := range map[string]string{"garbage": garbage, "wrong-order": mismatched, "foreign-key": foreign} {
		t.Run(name, func(t *testing.T) {
			tok := tok
			_, err := svc.ReturnDeposit(ctx, service.DepositReturnInput{
				OrderID: of.orderID,
				Token:   &tok,
				Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 1}},
				Actor:   opActor,
			})
			require.Error(t, err)
		})
	}

	require.Empty(t, prov.calls, "no rejected token may issue a refund")
	refs, err := repos.Refund.ListByOrder(ctx, of.orderID)
	require.NoError(t, err)
	require.Empty(t, refs, "no rejected token may persist a refund row")
}

// A deposit-line id can't be laundered through the generic line-refund endpoint.
func TestDepositReturn_LineRefundRejectsDepositLine(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	prov := &stubRefundProvider{}
	svc := newRefundSvc(repos, prov)

	txnID := 44
	of, _, _, depositLineID := createPaidDepositOrder(t, ctx, repos, paymentSvc, 1, 200, &txnID)

	_, err := svc.RefundLines(ctx, service.RefundLinesInput{
		OrderID: of.orderID,
		Lines:   []service.RefundLineQty{{OrderLineID: depositLineID, Quantity: 1}},
		Actor:   opActor,
	})
	require.Error(t, err)
	require.Empty(t, prov.calls)
}
