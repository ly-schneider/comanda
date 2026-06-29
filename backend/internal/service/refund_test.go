package service

import (
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
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
}

func TestRemainingPickupLines(t *testing.T) {
	lines := []redeemableLine{
		{OrderLineID: "l_simple", ProductID: "p_simple", LineType: orderline.LineTypeSimple, BoughtQty: 3},
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
	for _, l := range got {
		byProduct[l.ProductID] = l.Quantity
	}
	if byProduct["p_simple"] != 2 {
		t.Fatalf("simple line should have 2 remaining, got %d", byProduct["p_simple"])
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
