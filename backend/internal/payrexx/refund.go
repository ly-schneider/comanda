package payrexx

import (
	"context"
	"fmt"
	"net/http"
)

// RefundTransaction issues a (partial) refund against a confirmed transaction.
// amountCents is the amount to refund, not the remaining balance.
func (c *Client) RefundTransaction(ctx context.Context, transactionID int, amountCents int) error {
	endpoint := fmt.Sprintf("Transaction/%d/refund/", transactionID)
	_, err := c.doRequest(ctx, http.MethodPost, endpoint, map[string]any{"amount": amountCents})
	return err
}
