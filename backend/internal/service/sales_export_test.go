package service

import (
	"testing"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/orderpayment"
	"backend/internal/generated/ent/refund"
	"backend/internal/xlsx"
)

func TestAllocateCentsAlwaysSumsToTotal(t *testing.T) {
	cases := []struct {
		name    string
		total   int64
		weights []int64
		want    []int64
	}{
		{"three for two splits evenly", 6000, []int64{3000, 3000, 3000}, []int64{2000, 2000, 2000}},
		{"pro rata by list price", 4000, []int64{3000, 1000}, []int64{3000, 1000}},
		{"remainder goes to the largest fraction", 1000, []int64{1, 1, 1}, []int64{334, 333, 333}},
		{"priceless components split equally", 900, []int64{0, 0}, []int64{450, 450}},
		{"single component takes everything", 1250, []int64{500}, []int64{1250}},
		{"nothing to split", 0, []int64{100, 200}, []int64{0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := allocateCents(c.total, c.weights)
			var sum int64
			for i, v := range got {
				if v != c.want[i] {
					t.Fatalf("allocateCents(%d, %v) = %v, want %v", c.total, c.weights, got, c.want)
				}
				sum += v
			}
			if sum != c.total {
				t.Fatalf("parts sum to %d, want %d", sum, c.total)
			}
		})
	}
}

func TestBundleRevenueLandsOnTheComponents(t *testing.T) {
	catalog := testCatalog(
		testProduct("prod_shirt01", "Shirt", 3000, "cat_merch001"),
		testProduct("prod_drink01", "Getränk", 1000, "cat_merch001"),
	)
	// One bundle at CHF 32.00 holding a CHF 30.00 shirt and a CHF 10.00 drink.
	parent := testLine("line_parent1", orderline.LineTypeBundle, "prod_bundle1", "Shirt + Getränk", 1, 3200, nil)
	shirt := testLine("line_shirt01", orderline.LineTypeComponent, "prod_shirt01", "Shirt", 1, 0, &parent.ID)
	drink := testLine("line_drink01", orderline.LineTypeComponent, "prod_drink01", "Getränk", 1, 0, &parent.ID)
	ord := testOrder("ord_00000001", 3200, orderpayment.MethodTWINT, parent, shirt, drink)

	agg := newSalesAggregator(catalog)
	agg.addOrder(ord, orderRefunds{byLine: map[string]refundedLine{}})
	report := agg.finish(testReport())

	if len(report.Sales) != 2 {
		t.Fatalf("bundle must flatten to its components, got %d rows", len(report.Sales))
	}
	byProduct := map[string]int64{}
	for _, row := range report.Sales {
		byProduct[row.Product] = row.RevenueCents
		if !row.FromBundle {
			t.Fatalf("row %q should be marked as coming from a bundle", row.Product)
		}
		if row.Source != "Bundle: Shirt + Getränk" {
			t.Fatalf("Herkunft = %q", row.Source)
		}
	}
	if byProduct["Shirt"] != 2400 || byProduct["Getränk"] != 800 {
		t.Fatalf("pro-rata split = %v, want shirt 2400 / drink 800", byProduct)
	}
	if report.Totals.RevenueCents != 3200 {
		t.Fatalf("bundle revenue = %d, want the bundle price 3200 exactly once", report.Totals.RevenueCents)
	}
}

func TestBundleWithoutComponentsStaysItsOwnArticle(t *testing.T) {
	catalog := testCatalog(testProduct("prod_bundle1", "Wundertüte", 1500, "cat_merch001"))
	parent := testLine("line_parent1", orderline.LineTypeBundle, "prod_bundle1", "Wundertüte", 2, 1500, nil)
	ord := testOrder("ord_00000001", 3000, orderpayment.MethodCASH, parent)

	agg := newSalesAggregator(catalog)
	agg.addOrder(ord, orderRefunds{byLine: map[string]refundedLine{}})
	report := agg.finish(testReport())

	if len(report.Sales) != 1 || report.Sales[0].RevenueCents != 3000 {
		t.Fatalf("degenerate bundle must carry its own revenue, got %+v", report.Sales)
	}
	if report.Sales[0].FromBundle {
		t.Fatalf("a bundle without components is not a bundle line item")
	}
}

func TestGratisOrderIsZeroRevenueWithNominalValue(t *testing.T) {
	catalog := testCatalog(testProduct("prod_shirt01", "Shirt", 3000, "cat_merch001"))
	line := testLine("line_shirt01", orderline.LineTypeSimple, "prod_shirt01", "Shirt", 2, 3000, nil)
	ord := testOrder("ord_00000001", 6000, orderpayment.MethodGRATIS_VIP, line)

	agg := newSalesAggregator(catalog)
	agg.addOrder(ord, orderRefunds{byLine: map[string]refundedLine{}})
	report := agg.finish(testReport())

	row := report.Sales[0]
	if row.RevenueCents != 0 {
		t.Fatalf("gratis revenue = %d, want 0", row.RevenueCents)
	}
	if row.GratisCents != 6000 {
		t.Fatalf("gratis nominal value = %d, want 6000", row.GratisCents)
	}
	if row.PaymentMethod != "Gratis (VIP)" {
		t.Fatalf("payment label = %q", row.PaymentMethod)
	}
	if report.Totals.RevenueCents != 0 || report.Totals.GratisCents != 6000 {
		t.Fatalf("totals = %+v, want revenue 0 and gratis 6000", report.Totals)
	}
	if report.Totals.GratisOrderCount != 1 || report.Totals.GratisItemCount != 2 {
		t.Fatalf("gratis counters = %+v", report.Totals)
	}
	if report.Articles[0].GratisQty != 2 {
		t.Fatalf("article gratis quantity = %d, want 2", report.Articles[0].GratisQty)
	}
}

func TestDepositIsItsOwnColumnNotArticleRevenue(t *testing.T) {
	catalog := testCatalog(testProduct("prod_beer001", "Bier", 500, "cat_drinks01"))
	line := testLine("line_beer001", orderline.LineTypeSimple, "prod_beer001", "Bier", 2, 500, nil)
	deposit := testLine("line_dep0001", orderline.LineTypeDeposit, "prod_beer001", "Depot — Bier", 2, 200, &line.ID)
	ord := testOrder("ord_00000001", 1400, orderpayment.MethodCASH, line, deposit)

	agg := newSalesAggregator(catalog)
	agg.addOrder(ord, orderRefunds{byLine: map[string]refundedLine{}})
	report := agg.finish(testReport())

	if len(report.Sales) != 1 {
		t.Fatalf("a deposit is not an article of its own, got %d rows", len(report.Sales))
	}
	row := report.Sales[0]
	if row.RevenueCents != 1000 || row.DepositCents != 400 {
		t.Fatalf("row = revenue %d / deposit %d, want 1000 / 400", row.RevenueCents, row.DepositCents)
	}
	if got := report.NetCents(); got != 1400 {
		t.Fatalf("net takings = %d, want the order total 1400", got)
	}
}

func TestRefundsNetOutPerLineAndInTotal(t *testing.T) {
	catalog := testCatalog(testProduct("prod_shirt01", "Shirt", 3000, "cat_merch001"))
	line := testLine("line_shirt01", orderline.LineTypeSimple, "prod_shirt01", "Shirt", 3, 3000, nil)
	ord := testOrder("ord_00000001", 9000, orderpayment.MethodTWINT, line)
	ord.Status = order.StatusPartiallyRefunded

	refunds := indexRefunds([]*ent.Refund{
		testRefund("ord_00000001", 3000, refund.KindLine, testRefundLine("line_shirt01", 1, 3000)),
		testRefund("ord_00000001", 500, refund.KindArbitrary),
	})

	agg := newSalesAggregator(catalog)
	agg.addOrder(ord, refunds["ord_00000001"])
	report := agg.finish(testReport())

	row := report.Sales[0]
	if row.RefundedCents != 3000 || row.RefundedQty != 1 {
		t.Fatalf("line refund = %d cents / %d units, want 3000 / 1", row.RefundedCents, row.RefundedQty)
	}
	if report.Totals.RefundedCents != 3500 {
		t.Fatalf("total refunds = %d, want 3500 (line + arbitrary)", report.Totals.RefundedCents)
	}
	if report.Totals.UnlinkedRefunds != 500 {
		t.Fatalf("arbitrary refund = %d, want 500", report.Totals.UnlinkedRefunds)
	}
	if got := report.NetCents(); got != 5500 {
		t.Fatalf("net takings = %d, want 5500", got)
	}
	if report.Orders[0].RefundedCents != 3500 {
		t.Fatalf("receipt row refund = %d, want the full 3500", report.Orders[0].RefundedCents)
	}
}

func TestArticleAggregationSplitsDirectFromBundleSales(t *testing.T) {
	catalog := testCatalog(
		testProduct("prod_shirt01", "Shirt", 3000, "cat_merch001"),
		testProduct("prod_drink01", "Getränk", 1000, "cat_merch001"),
	)

	direct := testLine("line_shirt01", orderline.LineTypeSimple, "prod_shirt01", "Shirt", 2, 3000, nil)
	directOrder := testOrder("ord_00000001", 6000, orderpayment.MethodCASH, direct)

	parent := testLine("line_parent1", orderline.LineTypeBundle, "prod_bundle1", "Shirt + Getränk", 1, 3200, nil)
	shirt := testLine("line_shirt02", orderline.LineTypeComponent, "prod_shirt01", "Shirt", 1, 0, &parent.ID)
	drink := testLine("line_drink01", orderline.LineTypeComponent, "prod_drink01", "Getränk", 1, 0, &parent.ID)
	bundleOrder := testOrder("ord_00000002", 3200, orderpayment.MethodTWINT, parent, shirt, drink)

	agg := newSalesAggregator(catalog)
	agg.addOrder(directOrder, orderRefunds{byLine: map[string]refundedLine{}})
	agg.addOrder(bundleOrder, orderRefunds{byLine: map[string]refundedLine{}})
	report := agg.finish(testReport())

	var shirtArticle *xlsx.ArticleRow
	for i, a := range report.Articles {
		if a.Product == "Shirt" {
			shirtArticle = &report.Articles[i]
		}
	}
	if shirtArticle == nil {
		t.Fatal("no aggregated row for the shirt")
	}
	if shirtArticle.Quantity != 3 {
		t.Fatalf("shirt quantity = %d, want 3 (2 direct + 1 via bundle)", shirtArticle.Quantity)
	}
	if shirtArticle.BundleQty != 1 {
		t.Fatalf("shirt bundle quantity = %d, want 1", shirtArticle.BundleQty)
	}
	if shirtArticle.RevenueCents != 6000+2400 {
		t.Fatalf("shirt revenue = %d, want 8400", shirtArticle.RevenueCents)
	}
}

func TestPaymentMethodBreakdownStaysInCanonicalOrder(t *testing.T) {
	catalog := testCatalog(testProduct("prod_shirt01", "Shirt", 3000, "cat_merch001"))

	twintLine := testLine("line_shirt01", orderline.LineTypeSimple, "prod_shirt01", "Shirt", 1, 3000, nil)
	cashLine := testLine("line_shirt02", orderline.LineTypeSimple, "prod_shirt01", "Shirt", 1, 3000, nil)

	agg := newSalesAggregator(catalog)
	agg.addOrder(testOrder("ord_00000001", 3000, orderpayment.MethodTWINT, twintLine), orderRefunds{byLine: map[string]refundedLine{}})
	agg.addOrder(testOrder("ord_00000002", 3000, orderpayment.MethodCASH, cashLine), orderRefunds{byLine: map[string]refundedLine{}})
	report := agg.finish(testReport())

	if len(report.Methods) != 2 {
		t.Fatalf("expected both methods, got %+v", report.Methods)
	}
	if report.Methods[0].Method != "Bargeld" || report.Methods[1].Method != "TWINT" {
		t.Fatalf("method order = %q, %q — want cash first", report.Methods[0].Method, report.Methods[1].Method)
	}
	for _, m := range report.Methods {
		if m.RevenueCents != 3000 || m.OrderCount != 1 {
			t.Fatalf("method %q = %+v", m.Method, m)
		}
	}
}

// --- helpers ---

func testReport() xlsx.SalesReport { return xlsx.SalesReport{} }

func testCatalog(products ...*ent.Product) salesCatalog {
	c := salesCatalog{
		products:   make(map[string]*ent.Product, len(products)),
		variants:   make(map[string]*ent.ProductVariant),
		categories: make(map[string]*ent.Category),
	}
	for _, p := range products {
		c.products[p.ID] = p
	}
	return c
}

func testProduct(id, name string, priceCents int64, categoryID string) *ent.Product {
	p := &ent.Product{ID: id, Name: name, PriceCents: priceCents, CategoryID: categoryID}
	p.Edges.Category = &ent.Category{ID: categoryID, Name: "Merch"}
	return p
}

func testLine(id string, lineType orderline.LineType, productID, title string, quantity int, unitPriceCents int64, parentID *string) *ent.OrderLine {
	return &ent.OrderLine{
		ID:             id,
		LineType:       lineType,
		ProductID:      productID,
		Title:          title,
		Quantity:       quantity,
		UnitPriceCents: unitPriceCents,
		ParentLineID:   parentID,
	}
}

func testOrder(id string, totalCents int64, method orderpayment.Method, lines ...*ent.OrderLine) *ent.Order {
	o := &ent.Order{
		ID:         id,
		TotalCents: totalCents,
		Status:     order.StatusPaid,
		Origin:     order.OriginPos,
		CreatedAt:  time.Date(2026, 7, 24, 19, 30, 0, 0, time.UTC),
	}
	o.Edges.Lines = lines
	o.Edges.Payments = []*ent.OrderPayment{{
		OrderID:     id,
		Method:      method,
		AmountCents: totalCents,
		PaidAt:      o.CreatedAt,
	}}
	return o
}

func testRefund(orderID string, amountCents int64, kind refund.Kind, lines ...*ent.RefundLine) *ent.Refund {
	r := &ent.Refund{
		OrderID:     orderID,
		AmountCents: amountCents,
		Kind:        kind,
		Status:      refund.StatusSucceeded,
	}
	r.Edges.RefundLines = lines
	return r
}

func testRefundLine(orderLineID string, quantity int, amountCents int64) *ent.RefundLine {
	return &ent.RefundLine{OrderLineID: orderLineID, Quantity: quantity, AmountCents: amountCents}
}

func TestOrderWithoutPaymentStillLandsInTheCashUp(t *testing.T) {
	catalog := testCatalog(testProduct("prod_shirt01", "Shirt", 3000, "cat_merch001"))
	line := testLine("line_shirt01", orderline.LineTypeSimple, "prod_shirt01", "Shirt", 1, 3000, nil)
	ord := testOrder("ord_00000001", 3000, orderpayment.MethodCASH, line)
	ord.Edges.Payments = nil

	agg := newSalesAggregator(catalog)
	agg.addOrder(ord, orderRefunds{byLine: map[string]refundedLine{}})
	report := agg.finish(testReport())

	if len(report.Methods) != 1 || report.Methods[0].Method != unknownMethodLabel {
		t.Fatalf("methods = %+v, want a single Unbekannt bucket", report.Methods)
	}
	if report.Methods[0].RevenueCents != report.Totals.RevenueCents {
		t.Fatalf("cash-up %d does not add up to the total %d", report.Methods[0].RevenueCents, report.Totals.RevenueCents)
	}
}
