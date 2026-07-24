package payrexx

import (
	"encoding/json"
	"testing"
)

func TestSettleConfirmedGateway(t *testing.T) {
	body := []byte(`{"id":77,"status":"confirmed","referenceId":"ord_1","invoices":[{"transactions":[{"id":2012844,"status":"confirmed","contact":{"email":"gast@example.ch"}}]}]}`)
	var gw Gateway
	if err := json.Unmarshal(body, &gw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	s := gw.Settle()
	if !s.Paid || s.Failed {
		t.Fatalf("Settle() = %+v, want paid", s)
	}
	if s.TransactionID != 2012844 {
		t.Errorf("TransactionID = %d, want 2012844", s.TransactionID)
	}
	if s.ContactEmail != "gast@example.ch" {
		t.Errorf("ContactEmail = %q, want gast@example.ch", s.ContactEmail)
	}
}

// A gateway keeps every attempt it saw, so a retry after a decline must settle
// on the successful transaction — refunding the declined one would fail.
func TestSettlePrefersSuccessfulTransaction(t *testing.T) {
	body := []byte(`{"id":77,"status":"confirmed","invoices":[{"transactions":[{"id":1,"status":"declined"},{"id":2,"status":"confirmed","contact":{"email":"gast@example.ch"}}]}]}`)
	var gw Gateway
	if err := json.Unmarshal(body, &gw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	s := gw.Settle()
	if !s.Paid {
		t.Fatal("gateway with a confirmed transaction must be paid")
	}
	if s.TransactionID != 2 {
		t.Errorf("TransactionID = %d, want 2 (the confirmed one)", s.TransactionID)
	}
}

// The customer's order matters more than our ability to refund it later, so a
// confirmed gateway with no recoverable transaction id still settles.
func TestSettleConfirmedWithoutTransactionID(t *testing.T) {
	var gw Gateway
	if err := json.Unmarshal([]byte(`{"id":77,"status":"confirmed"}`), &gw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	s := gw.Settle()
	if !s.Paid {
		t.Fatal("confirmed gateway must be paid even without a transaction id")
	}
	if s.TransactionID != 0 {
		t.Errorf("TransactionID = %d, want 0", s.TransactionID)
	}
}

func TestSettleTopLevelTransactionID(t *testing.T) {
	var gw Gateway
	if err := json.Unmarshal([]byte(`{"id":77,"status":"confirmed","transactionId":991}`), &gw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if s := gw.Settle(); s.TransactionID != 991 {
		t.Errorf("TransactionID = %d, want 991", s.TransactionID)
	}
}

func TestSettleNonTerminalAndFailedStatuses(t *testing.T) {
	cases := map[string]struct{ paid, failed bool }{
		GatewayStatusWaiting:   {false, false},
		GatewayStatusConfirmed: {true, false},
		GatewayStatusCancelled: {false, true},
		GatewayStatusDeclined:  {false, true},
		GatewayStatusError:     {false, true},
		GatewayStatusExpired:   {false, true},
	}
	for status, want := range cases {
		gw := Gateway{Status: status}
		s := gw.Settle()
		if s.Paid != want.paid || s.Failed != want.failed {
			t.Errorf("Settle(%q) = paid %v failed %v, want paid %v failed %v",
				status, s.Paid, s.Failed, want.paid, want.failed)
		}
	}
}

// A gateway can report "waiting" while carrying a confirmed transaction; the
// money is in and the order must not be left pending.
func TestSettleWaitingGatewayWithConfirmedTransaction(t *testing.T) {
	var gw Gateway
	if err := json.Unmarshal([]byte(`{"id":77,"status":"waiting","invoices":[{"transactions":[{"id":5,"status":"confirmed"}]}]}`), &gw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	s := gw.Settle()
	if !s.Paid || s.Failed {
		t.Fatalf("Settle() = %+v, want paid", s)
	}
	if s.TransactionID != 5 {
		t.Errorf("TransactionID = %d, want 5", s.TransactionID)
	}
}
