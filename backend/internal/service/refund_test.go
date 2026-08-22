package service

import (
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/orderpayment"
	"backend/internal/generated/ent/refund"
)

func intPtr(v int) *int { return &v }

func TestRefundMethodFor(t *testing.T) {
	if got := refundMethodFor(intPtr(42)); got != refund.MethodTwint {
		t.Fatalf("payrexx-backed order should route to twint, got %s", got)
	}
	if got := refundMethodFor(nil); got != refund.MethodCash {
		t.Fatalf("card/cash order should cash-out, got %s", got)
	}
}

func TestRefundableBalanceCents(t *testing.T) {
	cases := []struct {
		total, succeeded, deposit, want int64
	}{
		{1000, 0, 0, 1000},
		{1000, 400, 0, 600},
		{1000, 1000, 0, 0},
		{1000, 1200, 0, 0},    // over-refunded clamps to 0, never negative
		{1000, 200, 300, 500}, // deposit balance held out of the product-refundable balance
	}
	for _, c := range cases {
		if got := refundableBalanceCents(c.total, c.succeeded, c.deposit); got != c.want {
			t.Fatalf("refundableBalanceCents(%d,%d,%d)=%d want %d", c.total, c.succeeded, c.deposit, got, c.want)
		}
	}
}

func TestOrderStatusAfterRefund(t *testing.T) {
	if got := orderStatusAfterRefund(1000, 1000); got != order.StatusRefunded {
		t.Fatalf("balance to zero must be refunded, got %s", got)
	}
	if got := orderStatusAfterRefund(1000, 1100); got != order.StatusRefunded {
		t.Fatalf("over-full must be refunded, got %s", got)
	}
	if got := orderStatusAfterRefund(1000, 600); got != order.StatusPartiallyRefunded {
		t.Fatalf("partial must be partially_refunded, got %s", got)
	}
}

func gratisOrderFixture(method orderpayment.Method) *ent.Order {
	parent := "p1"
	ord := &ent.Order{TotalCents: 1100, Edges: ent.OrderEdges{Lines: []*ent.OrderLine{
		{ID: "p1", LineType: orderline.LineTypeSimple, Quantity: 2, UnitPriceCents: 350},
		{ID: "d1", LineType: orderline.LineTypeDeposit, Quantity: 2, UnitPriceCents: 200, ParentLineID: &parent},
	}}}
	ord.Edges.Payments = []*ent.OrderPayment{{Method: method, AmountCents: ord.TotalCents}}
	return ord
}

func TestCollectedCents(t *testing.T) {
	paid := gratisOrderFixture(orderpayment.MethodCASH)
	if got := collectedCents(paid); got != 1100 {
		t.Fatalf("a paid order collected its total, got %d", got)
	}
	if got := refundUnitPrice(paid, paid.Edges.Lines[0]); got != 350 {
		t.Fatalf("a paid line refunds its price, got %d", got)
	}

	for _, method := range []orderpayment.Method{
		orderpayment.MethodGRATIS_GUEST,
		orderpayment.MethodGRATIS_VIP,
		orderpayment.MethodGRATIS_STAFF,
		orderpayment.MethodGRATIS_PLUGIN,
	} {
		comp := gratisOrderFixture(method)
		if got := collectedCents(comp); got != 0 {
			t.Fatalf("%s took no money, got %d", method, got)
		}
		if got := refundUnitPrice(comp, comp.Edges.Lines[0]); got != 0 {
			t.Fatalf("%s refunds nothing per unit, got %d", method, got)
		}
		// The deposit is never collected on a comp either, so it holds nothing back.
		if got := outstandingDepositCents(comp, map[string]int{}); got != 0 {
			t.Fatalf("%s holds no deposit balance, got %d", method, got)
		}
	}

	// An order with no payment row yet (still pending) keeps its nominal total.
	if got := collectedCents(&ent.Order{TotalCents: 500}); got != 500 {
		t.Fatalf("an unpaid order is not a comp, got %d", got)
	}
}

func TestOrderStatusAfterLineRefund(t *testing.T) {
	if got := orderStatusAfterLineRefund(1000, 600, 1); got != order.StatusPartiallyRefunded {
		t.Fatalf("a paid order still follows the money, got %s", got)
	}
	if got := orderStatusAfterLineRefund(1000, 1000, 3); got != order.StatusRefunded {
		t.Fatalf("money back settles a paid order even with units out, got %s", got)
	}
	// A comp moves no money, so only the units returned can complete it.
	if got := orderStatusAfterLineRefund(0, 0, 1); got != order.StatusPartiallyRefunded {
		t.Fatalf("a comp with a unit still out is partial, got %s", got)
	}
	if got := orderStatusAfterLineRefund(0, 0, 0); got != order.StatusRefunded {
		t.Fatalf("a comp with every unit back is refunded, got %s", got)
	}
}

func TestUnitsOutstanding(t *testing.T) {
	parent := "b1"
	ord := &ent.Order{Edges: ent.OrderEdges{Lines: []*ent.OrderLine{
		{ID: "p1", LineType: orderline.LineTypeSimple, Quantity: 2},
		{ID: "b1", LineType: orderline.LineTypeBundle, Quantity: 1},
		{ID: "c1", LineType: orderline.LineTypeComponent, Quantity: 1, ParentLineID: &parent},
		{ID: "d1", LineType: orderline.LineTypeDeposit, Quantity: 2, ParentLineID: &parent},
	}}}

	if got := unitsOutstanding(ord, map[string]int{}); got != 3 {
		t.Fatalf("only top-level product units count, got %d", got)
	}
	if got := unitsOutstanding(ord, map[string]int{"p1": 1}); got != 2 {
		t.Fatalf("refunded units drop out, got %d", got)
	}
	if got := unitsOutstanding(ord, map[string]int{"p1": 2, "b1": 1}); got != 0 {
		t.Fatalf("everything back leaves nothing outstanding, got %d", got)
	}
	if got := unitsOutstanding(ord, map[string]int{"p1": 5}); got != 1 {
		t.Fatalf("an over-refunded line must not go negative, got %d", got)
	}
}

func TestReconcilableRefundDelta(t *testing.T) {
	cases := []struct {
		name                                   string
		total, providerRefunded, alreadyRecord int64
		want                                   int64
	}{
		{"partial refund, nothing recorded", 1000, 300, 0, 300},
		{"partial refund already recorded", 1000, 300, 300, 0},
		{"external top-up beyond ledger", 1000, 500, 300, 200},
		{"full refund, nothing recorded", 1000, 1000, 0, 1000},
		{"over-total clamps to total", 1000, 1500, 0, 1000},
		{"ledger exceeds provider", 1000, 300, 400, 0},
	}
	for _, c := range cases {
		if got := reconcilableRefundDelta(c.total, c.providerRefunded, c.alreadyRecord); got != c.want {
			t.Fatalf("%s: reconcilableRefundDelta(%d,%d,%d)=%d want %d",
				c.name, c.total, c.providerRefunded, c.alreadyRecord, got, c.want)
		}
	}
}

func TestCappedArbitraryAmount(t *testing.T) {
	if got := cappedArbitraryAmount(900, 500); got != 500 {
		t.Fatalf("over-cap request must be capped at the balance, got %d", got)
	}
	if got := cappedArbitraryAmount(300, 500); got != 300 {
		t.Fatalf("within-cap request must pass through, got %d", got)
	}
}

func TestValidateRefundLineQty(t *testing.T) {
	if err := validateRefundLineQty(3, 0, 0); err == nil {
		t.Fatal("zero quantity must be rejected")
	}
	if err := validateRefundLineQty(3, 2, 2); err == nil {
		t.Fatal("2 already refunded + 2 more on a qty-3 line must be rejected")
	}
	if err := validateRefundLineQty(3, 2, 1); err != nil {
		t.Fatalf("the last unit must be refundable, got %v", err)
	}
	if err := validateRefundLineQty(3, 0, 3); err != nil {
		t.Fatalf("full quantity must be refundable, got %v", err)
	}
}

func TestOutstandingDepositCents(t *testing.T) {
	ord := &ent.Order{Edges: ent.OrderEdges{Lines: []*ent.OrderLine{
		{ID: "p1", LineType: orderline.LineTypeSimple, Quantity: 2, UnitPriceCents: 350},
		{ID: "d1", LineType: orderline.LineTypeDeposit, Quantity: 2, UnitPriceCents: 200},
	}}}

	if got := outstandingDepositCents(ord, map[string]int{}); got != 400 {
		t.Fatalf("two unreturned deposits = 400, got %d", got)
	}
	if got := outstandingDepositCents(ord, map[string]int{"d1": 1}); got != 200 {
		t.Fatalf("one returned leaves 200 outstanding, got %d", got)
	}
	if got := outstandingDepositCents(ord, map[string]int{"d1": 2}); got != 0 {
		t.Fatalf("all returned leaves 0 outstanding, got %d", got)
	}
	// Product-line refunds never reduce the deposit balance.
	if got := outstandingDepositCents(ord, map[string]int{"p1": 2}); got != 400 {
		t.Fatalf("product refunds must not touch the deposit balance, got %d", got)
	}
}

func TestLinkedDepositSpecs(t *testing.T) {
	parent := "p1"
	ord := &ent.Order{Edges: ent.OrderEdges{Lines: []*ent.OrderLine{
		{ID: "p1", LineType: orderline.LineTypeSimple, ProductID: "prod", Quantity: 2, UnitPriceCents: 350},
		{ID: "d1", LineType: orderline.LineTypeDeposit, ProductID: "prod", Quantity: 2, UnitPriceCents: 200, ParentLineID: &parent},
	}}}
	productSpecs := []refundLineSpec{{orderLineID: "p1", lineType: orderline.LineTypeSimple, quantity: 1}}

	got := linkedDepositSpecs(ord, productSpecs, map[string]int{})
	if len(got) != 1 {
		t.Fatalf("refunding one product unit releases one deposit spec, got %d", len(got))
	}
	if got[0].orderLineID != "d1" || got[0].quantity != 1 || got[0].amountCents != 200 || got[0].lineType != orderline.LineTypeDeposit {
		t.Fatalf("unexpected linked deposit spec: %+v", got[0])
	}

	// A deposit already returned via the return flow is capped to zero.
	if got := linkedDepositSpecs(ord, productSpecs, map[string]int{"d1": 2}); len(got) != 0 {
		t.Fatalf("already-returned deposit must not be released again, got %+v", got)
	}

	// Duplicate specs for the same product line must not over-release: with one
	// unit already returned, two {p1,1} specs release only the single remaining unit.
	dup := []refundLineSpec{
		{orderLineID: "p1", lineType: orderline.LineTypeSimple, quantity: 1},
		{orderLineID: "p1", lineType: orderline.LineTypeSimple, quantity: 1},
	}
	total := 0
	for _, s := range linkedDepositSpecs(ord, dup, map[string]int{"d1": 1}) {
		total += s.quantity
	}
	if total != 1 {
		t.Fatalf("duplicate product specs must release only the 1 remaining deposit unit, got %d", total)
	}
}

func TestRemainingPickupLines(t *testing.T) {
	lines := []redeemableLine{
		{OrderLineID: "l_simple", ProductID: "p_simple", VariantID: "vrnt_size_xl", LineType: orderline.LineTypeSimple, BoughtQty: 3},
		{OrderLineID: "l_redeemed", ProductID: "p_redeemed", LineType: orderline.LineTypeSimple, BoughtQty: 2, Redeemed: true},
		{OrderLineID: "l_bundle", ProductID: "p_bundle", LineType: orderline.LineTypeBundle, BoughtQty: 1, HasChildren: true},
		{OrderLineID: "l_component", ProductID: "p_component", LineType: orderline.LineTypeComponent, BoughtQty: 1},
	}

	got := remainingPickupLines(lines, map[string]int{"l_simple": 1})

	// l_simple: 3 bought - 1 refunded = 2 remaining.
	// l_redeemed: already handed over -> excluded (money-only).
	// l_bundle: configured parent carries no pickup item -> excluded.
	// l_component: 1 remaining.
	if len(got) != 2 {
		t.Fatalf("expected 2 remaining pickup lines, got %d (%+v)", len(got), got)
	}
	byProduct := map[string]int{}
	variantByProduct := map[string]string{}
	for _, l := range got {
		byProduct[l.ProductID] = l.Quantity
		variantByProduct[l.ProductID] = l.VariantID
	}
	if byProduct["p_simple"] != 2 {
		t.Fatalf("simple line should have 2 remaining, got %d", byProduct["p_simple"])
	}
	if variantByProduct["p_simple"] != "vrnt_size_xl" {
		t.Fatalf("re-issued pickup line must carry the variant, got %q", variantByProduct["p_simple"])
	}
	if byProduct["p_component"] != 1 {
		t.Fatalf("component line should have 1 remaining, got %d", byProduct["p_component"])
	}

	// Refunding the simple line down to zero drops it entirely.
	got = remainingPickupLines(lines, map[string]int{"l_simple": 3})
	for _, l := range got {
		if l.ProductID == "p_simple" {
			t.Fatal("fully-refunded simple line must be absent from the pickup set")
		}
	}
}
