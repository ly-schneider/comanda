package payrexx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RefundTransaction issues a (partial) refund against a confirmed transaction.
// amountCents is the amount to refund, not the remaining balance.
func (c *Client) RefundTransaction(ctx context.Context, transactionID int, amountCents int) error {
	endpoint := fmt.Sprintf("Transaction/%d/refund/", transactionID)
	body, err := c.doRequest(ctx, http.MethodPost, endpoint, map[string]any{"amount": amountCents})
	if err != nil {
		return err
	}
	// A 2xx alone is not proof of success: Payrexx signals a rejected refund with
	// an "error" status in the body. Surface it so the caller rolls the refund
	// back instead of recording money that never moved. An absent status (unusual
	// payload shape) is left as success to avoid failing a refund that did apply.
	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("payrexx: failed to parse refund response: %w", err)
	}
	if resp.Status != "" && resp.Status != "success" {
		msg := resp.Message
		if msg == "" {
			msg = "refund rejected"
		}
		return fmt.Errorf("payrexx: refund failed: %s", msg)
	}
	return nil
}
