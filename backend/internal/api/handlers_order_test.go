package api

import (
	"context"
	"testing"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/payment"
	"backend/internal/service"
)

// fakeRefundService stubs RefundService; only RefundedSummary is exercised by
// these tests, the rest panic to flag any unexpected call.
type fakeRefundService struct {
	summary service.RefundedSummary
	err     error
}

func (f *fakeRefundService) RefundedSummary(context.Context, string) (service.RefundedSummary, error) {
	return f.summary, f.err
}
func (f *fakeRefundService) GetRefundView(context.Context, string) (*service.RefundView, error) {
	panic("unexpected")
}
func (f *fakeRefundService) ListRefunds(context.Context, int, int) ([]*ent.Refund, error) {
	panic("unexpected")
}
func (f *fakeRefundService) LookupForPOS(context.Context, string) ([]*ent.Order, error) {
	panic("unexpected")
}
func (f *fakeRefundService) RefundLines(context.Context, service.RefundLinesInput) (*ent.Refund, error) {
	panic("unexpected")
}
func (f *fakeRefundService) RefundArbitrary(context.Context, service.RefundArbitraryInput) (*ent.Refund, error) {
	panic("unexpected")
}
func (f *fakeRefundService) ReturnDeposit(context.Context, service.DepositReturnInput) (*ent.Refund, error) {
	panic("unexpected")
}
func (f *fakeRefundService) CancelOrder(context.Context, service.CancelOrderInput) (*ent.Refund, error) {
	panic("unexpected")
}
func (f *fakeRefundService) ReconcileFromWebhook(context.Context, *payment.WebhookResult) error {
	panic("unexpected")
}

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

func TestAnnotateOrderRefunds(t *testing.T) {
	t.Run("sets total and line quantities when refunded", func(t *testing.T) {
		h := &Handlers{refunds: &fakeRefundService{
			summary: service.RefundedSummary{TotalCents: 200, ByLine: map[string]int{"l1": 1}},
		}}
		o := &generated.Order{Id: "o1", Lines: &[]generated.OrderLine{{Id: "l1"}}}
		if err := h.annotateOrderRefunds(context.Background(), o); err != nil {
			t.Fatalf("annotateOrderRefunds: %v", err)
		}
		if o.RefundedTotalCents == nil || *o.RefundedTotalCents != 200 {
			t.Errorf("refundedTotalCents = %v, want 200", o.RefundedTotalCents)
		}
		if l := (*o.Lines)[0]; l.RefundedQuantity == nil || *l.RefundedQuantity != 1 {
			t.Errorf("line refundedQuantity = %v, want 1", l.RefundedQuantity)
		}
	})

	t.Run("omits total when zero", func(t *testing.T) {
		h := &Handlers{refunds: &fakeRefundService{summary: service.RefundedSummary{TotalCents: 0}}}
		o := &generated.Order{Id: "o1", Lines: &[]generated.OrderLine{{Id: "l1"}}}
		if err := h.annotateOrderRefunds(context.Background(), o); err != nil {
			t.Fatalf("annotateOrderRefunds: %v", err)
		}
		if o.RefundedTotalCents != nil {
			t.Errorf("refundedTotalCents = %v, want nil", o.RefundedTotalCents)
		}
	})

	t.Run("tolerates nil lines", func(t *testing.T) {
		h := &Handlers{refunds: &fakeRefundService{summary: service.RefundedSummary{TotalCents: 200}}}
		o := &generated.Order{Id: "o1"}
		if err := h.annotateOrderRefunds(context.Background(), o); err != nil {
			t.Fatalf("annotateOrderRefunds: %v", err)
		}
		if o.RefundedTotalCents == nil || *o.RefundedTotalCents != 200 {
			t.Errorf("refundedTotalCents = %v, want 200", o.RefundedTotalCents)
		}
	})

	t.Run("propagates the lookup error", func(t *testing.T) {
		h := &Handlers{refunds: &fakeRefundService{err: context.DeadlineExceeded}}
		o := &generated.Order{Id: "o1", Lines: &[]generated.OrderLine{{Id: "l1"}}}
		if err := h.annotateOrderRefunds(context.Background(), o); err == nil {
			t.Fatal("expected error, got nil")
		}
		if o.RefundedTotalCents != nil {
			t.Errorf("refundedTotalCents = %v, want nil on error", o.RefundedTotalCents)
		}
	})
}
