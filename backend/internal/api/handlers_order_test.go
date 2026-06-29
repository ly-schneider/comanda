package api

import (
	"testing"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent/order"
)

func TestOrderReflectsRefunds(t *testing.T) {
	cases := map[order.Status]bool{
		order.StatusPending:           false,
		order.StatusPaid:              false,
		order.StatusRefunded:          true,
		order.StatusPartiallyRefunded: true,
		order.StatusCancelled:         true,
	}
	for status, want := range cases {
		if got := orderReflectsRefunds(status); got != want {
			t.Errorf("orderReflectsRefunds(%s) = %v, want %v", status, got, want)
		}
	}
}

func TestAnnotateLineRefunds(t *testing.T) {
	lines := []generated.OrderLine{
		{Id: "prod1"},
		{Id: "bundle1", ChildLines: &[]generated.OrderLine{
			{Id: "comp1"},
			{Id: "deposit1"},
		}},
		{Id: "prod2"},
	}
	byLine := map[string]int{"prod1": 1, "deposit1": 2, "prod2": 0}

	annotateLineRefunds(lines, byLine)

	if lines[0].RefundedQuantity == nil || *lines[0].RefundedQuantity != 1 {
		t.Errorf("prod1 refundedQuantity = %v, want 1", lines[0].RefundedQuantity)
	}
	// Zero-quantity entries stay unset so the field is omitted from the response.
	if lines[2].RefundedQuantity != nil {
		t.Errorf("prod2 refundedQuantity = %v, want nil", lines[2].RefundedQuantity)
	}
	children := *lines[1].ChildLines
	if children[0].RefundedQuantity != nil {
		t.Errorf("comp1 refundedQuantity = %v, want nil", children[0].RefundedQuantity)
	}
	if children[1].RefundedQuantity == nil || *children[1].RefundedQuantity != 2 {
		t.Errorf("deposit1 refundedQuantity = %v, want 2", children[1].RefundedQuantity)
	}
}
