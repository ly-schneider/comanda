package integration

import (
	"context"
	"testing"
	"time"

	entOrder "backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/orderpayment"
	entProduct "backend/internal/generated/ent/product"
	"backend/internal/generated/ent/refund"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/xlsx"

	"github.com/stretchr/testify/require"
)

// A full round trip over the real schema: the range query, the eager-loaded
// lines and payments, and the money that has to come back out of them.
func TestSalesExport_RevenuePerArticleAcrossBundlesAndRefunds(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	ctx := context.Background()

	svc := service.NewSalesExportService(repos.Order, repos.Refund, repos.Product, repos.Category)

	merch := fixtures.CreateCategory("Merch", 1, true)
	shirt := fixtures.CreateProduct("Shirt", merch.ID, 3000, entProduct.TypeSimple, nil)
	drink := fixtures.CreateProduct("Getränk", merch.ID, 1000, entProduct.TypeSimple, nil)
	combo := fixtures.CreateProduct("Shirt + Getränk", merch.ID, 3200, entProduct.TypeBundle, nil)

	// Paid by card: two shirts straight off the shelf, one of them refunded.
	direct := fixtures.CreateOrder(6000, entOrder.StatusPartiallyRefunded, entOrder.OriginPos)
	directLine := fixtures.CreateOrderLine(direct.ID, shirt.ID, "Shirt", 2, 3000, orderline.LineTypeSimple)
	payOrder(t, repos, direct.ID, orderpayment.MethodCARD, 6000)
	refundLine(t, repos, direct.ID, directLine.ID, 1, 3000)

	// Paid by TWINT: the bundle, whose price has to land on its components.
	bundleOrder := fixtures.CreateOrder(3200, entOrder.StatusPaid, entOrder.OriginShop)
	parent := fixtures.CreateOrderLine(bundleOrder.ID, combo.ID, "Shirt + Getränk", 1, 3200, orderline.LineTypeBundle)
	addChild(t, repos, bundleOrder.ID, shirt.ID, "Shirt", parent.ID)
	addChild(t, repos, bundleOrder.ID, drink.ID, "Getränk", parent.ID)
	payOrder(t, repos, bundleOrder.ID, orderpayment.MethodTWINT, 3200)

	// A VIP giveaway: nominal value only, no takings.
	vip := fixtures.CreateOrder(3000, entOrder.StatusPaid, entOrder.OriginPos)
	fixtures.CreateOrderLine(vip.ID, shirt.ID, "Shirt", 1, 3000, orderline.LineTypeSimple)
	payOrder(t, repos, vip.ID, orderpayment.MethodGRATIS_VIP, 3000)

	// A pending order never took money and must stay out of the report.
	pending := fixtures.CreateOrder(3000, entOrder.StatusPending, entOrder.OriginShop)
	fixtures.CreateOrderLine(pending.ID, shirt.ID, "Shirt", 1, 3000, orderline.LineTypeSimple)

	report, err := svc.Build(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)

	require.Equal(t, 3, report.Totals.OrderCount, "pending orders must not be reported")

	var shirtRow, drinkRow *xlsx.ArticleRow
	for i := range report.Articles {
		switch report.Articles[i].Product {
		case "Shirt":
			shirtRow = &report.Articles[i]
		case "Getränk":
			drinkRow = &report.Articles[i]
		}
	}
	require.NotNil(t, shirtRow)
	require.NotNil(t, drinkRow)

	// 2 sold directly + 1 via the bundle + 1 given away.
	require.Equal(t, 4, shirtRow.Quantity)
	require.Equal(t, 1, shirtRow.BundleQty)
	require.Equal(t, 1, shirtRow.GratisQty)
	require.Equal(t, 1, shirtRow.RefundedQty)
	// CHF 60 direct + the shirt's CHF 24 share of the CHF 32 bundle; the VIP shirt
	// is worth CHF 30 but earned nothing.
	require.Equal(t, int64(8400), shirtRow.RevenueCents)
	require.Equal(t, int64(3000), shirtRow.RefundedCents)
	require.Equal(t, int64(3000), shirtRow.GratisCents)

	require.Equal(t, int64(800), drinkRow.RevenueCents, "the drink takes the rest of the bundle price")

	require.Equal(t, int64(9200), report.Totals.RevenueCents)
	require.Equal(t, int64(3000), report.Totals.RefundedCents)
	require.Equal(t, int64(6200), report.NetCents())
	require.Equal(t, int64(3000), report.Totals.GratisCents)

	byMethod := map[string]int64{}
	for _, m := range report.Methods {
		byMethod[m.Method] = m.NetCents()
	}
	require.Equal(t, int64(3000), byMethod["Karte"], "card takings net of the refund")
	require.Equal(t, int64(3200), byMethod["TWINT"])
	require.Equal(t, int64(0), byMethod["Gratis (VIP)"])
}

func TestSalesExport_RejectsInvertedRange(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	svc := service.NewSalesExportService(repos.Order, repos.Refund, repos.Product, repos.Category)

	now := time.Now()
	_, err := svc.Build(context.Background(), now, now.Add(-time.Hour))
	require.ErrorIs(t, err, service.ErrInvalidExportRange)
}

func addChild(t *testing.T, repos *Repositories, orderID, productID, title, parentID string) {
	t.Helper()
	_, err := repos.OrderLine.Create(
		context.Background(), orderID, orderline.LineTypeComponent, productID, title, 1, 0, &parentID, nil, nil,
	)
	require.NoError(t, err)
}

func payOrder(t *testing.T, repos *Repositories, orderID string, method orderpayment.Method, amountCents int64) {
	t.Helper()
	_, err := repos.OrderPayment.Create(context.Background(), orderID, method, amountCents, time.Now(), nil)
	require.NoError(t, err)
}

func refundLine(t *testing.T, repos *Repositories, orderID, orderLineID string, quantity int, amountCents int64) {
	t.Helper()
	ctx := context.Background()
	created, err := repos.Refund.CreateRefund(ctx, repository.CreateRefundParams{
		OrderID:     orderID,
		AmountCents: amountCents,
		Kind:        refund.KindLine,
		Method:      refund.MethodCash,
		ActorType:   refund.ActorTypeAdmin,
		ActorID:     "usr_00000001",
		Status:      refund.StatusSucceeded,
	})
	require.NoError(t, err)
	_, err = repos.Refund.CreateRefundLines(ctx, []repository.CreateRefundLineParams{{
		RefundID:    created.ID,
		OrderLineID: orderLineID,
		Quantity:    quantity,
		AmountCents: amountCents,
	}})
	require.NoError(t, err)
}
