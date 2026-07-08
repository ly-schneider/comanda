package payrexx

import "testing"

func TestParseWebhookRefundAmounts(t *testing.T) {
	// A partial refund carries the original charge in `amount` and the cumulative
	// refunded portion in `refundedAmount`; reconciling off `amount` is the
	// LEY-387 escalation bug, so callers must read RefundedAmount instead.
	body := []byte(`{"transaction":{"id":42,"status":"partially-refunded","amount":1000,"originalAmount":1000,"refundedAmount":300,"referenceId":"ord_1"}}`)
	event, err := ParseWebhookEvent(body)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	txn := event.Transaction
	if txn.Amount != 1000 {
		t.Errorf("Amount = %d, want 1000", txn.Amount)
	}
	if txn.RefundedAmount != 300 {
		t.Errorf("RefundedAmount = %d, want 300", txn.RefundedAmount)
	}
	if !txn.IsRefundStatus() {
		t.Error("partially-refunded must be a refund status")
	}
	if txn.IsFullRefundStatus() {
		t.Error("partially-refunded must not be a full-refund status")
	}
}

func TestIsFullRefundStatus(t *testing.T) {
	full := map[string]bool{
		TransactionStatusRefunded:   true,
		TransactionStatusChargeback: true,
		TransactionStatusPartRefund: false,
		TransactionStatusConfirmed:  false,
	}
	for status, want := range full {
		txn := WebhookTransaction{Status: status}
		if got := txn.IsFullRefundStatus(); got != want {
			t.Errorf("IsFullRefundStatus(%q) = %v, want %v", status, got, want)
		}
	}
}
