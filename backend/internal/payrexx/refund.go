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
	// Give the slow refund endpoint its own longer budget before doRequest would
	// otherwise clamp it to the 8s DefaultRequestTimeout.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, RefundRequestTimeout)
		defer cancel()
	}
	endpoint := fmt.Sprintf("Transaction/%d/refund/", transactionID)
	body, err := c.doRequest(ctx, http.MethodPost, endpoint, map[string]any{"amount": amountCents})
	if err != nil {
		return err
	}
	// A 2xx can still carry a Payrexx "error" status; surface it so a rejected
	// refund isn't recorded as money moved. An empty or status-less body is a
	// success — don't fail a refund that applied.
	if len(body) == 0 {
		return nil
	}
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
