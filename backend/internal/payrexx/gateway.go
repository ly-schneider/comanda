package payrexx

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Gateway struct {
	ID           int       `json:"id"`
	Status       string    `json:"status"`
	Hash         string    `json:"hash"`
	Link         string    `json:"link"`
	Amount       int       `json:"amount"`
	Currency     string    `json:"currency"`
	ReferenceID  string    `json:"referenceId"`
	PaymentMeans []string  `json:"pm"`
	CreatedAt    time.Time `json:"-"`
	// Only some responses carry transactionId; otherwise it's inside the invoices.
	TransactionID *int             `json:"transactionId,omitempty"`
	Invoices      []GatewayInvoice `json:"invoices,omitempty"`
}

type GatewayInvoice struct {
	Transactions []GatewayTransaction `json:"transactions,omitempty"`
}

type GatewayTransaction struct {
	ID      int            `json:"id"`
	Status  string         `json:"status"`
	Contact WebhookContact `json:"contact,omitempty"`
}

type Settlement struct {
	Paid          bool
	Failed        bool
	TransactionID int
	ContactEmail  string
}

// Settle reads a pulled gateway the way the webhook path reads a pushed
// transaction. A confirmed gateway with no recoverable transaction id is still
// Paid — the customer gets their order; refunding it then needs the dashboard.
func (g *Gateway) Settle() Settlement {
	s := Settlement{}
	switch g.Status {
	case GatewayStatusConfirmed:
		s.Paid = true
	case GatewayStatusCancelled, GatewayStatusDeclined, GatewayStatusError, GatewayStatusExpired:
		s.Failed = true
	}

	for _, inv := range g.Invoices {
		for _, txn := range inv.Transactions {
			if txn.ID == 0 {
				continue
			}
			// A decline followed by a successful retry leaves both on the gateway.
			if s.TransactionID == 0 || isSuccessTransactionStatus(txn.Status) {
				s.TransactionID = txn.ID
				if txn.Contact.Email != "" {
					s.ContactEmail = txn.Contact.Email
				}
			}
			if isSuccessTransactionStatus(txn.Status) {
				s.Paid = true
				s.Failed = false
			}
		}
	}

	if s.TransactionID == 0 && g.TransactionID != nil {
		s.TransactionID = *g.TransactionID
	}
	return s
}

func isSuccessTransactionStatus(status string) bool {
	return status == TransactionStatusConfirmed ||
		status == TransactionStatusAuthorized ||
		status == TransactionStatusReserved
}

type InvoiceItem struct {
	Name     string
	Quantity int
	Amount   int
}

type GatewayResponse struct {
	Status  string    `json:"status"`
	Message string    `json:"message,omitempty"`
	Data    []Gateway `json:"data"`
}

type CreateGatewayParams struct {
	Amount             int
	Currency           string
	ReferenceID        string
	SuccessRedirectURL string
	FailedRedirectURL  string
	CancelRedirectURL  string
	PaymentMeans       []string
	InvoiceItems       []InvoiceItem
	CustomerEmail      string
	Purpose            string
	ValidityMinutes    int
	LookAndFeelProfile string
}

func (c *Client) CreateGateway(ctx context.Context, params CreateGatewayParams) (*Gateway, error) {
	p := map[string]any{
		"amount":   params.Amount,
		"currency": params.Currency,
	}

	if params.ReferenceID != "" {
		p["referenceId"] = params.ReferenceID
	}
	if params.SuccessRedirectURL != "" {
		p["successRedirectUrl"] = params.SuccessRedirectURL
	}
	if params.FailedRedirectURL != "" {
		p["failedRedirectUrl"] = params.FailedRedirectURL
	}
	if params.CancelRedirectURL != "" {
		p["cancelRedirectUrl"] = params.CancelRedirectURL
	}
	if params.CustomerEmail != "" {
		p["fields"] = map[string]any{
			"email": map[string]any{
				"value": params.CustomerEmail,
			},
		}
	}
	if params.Purpose != "" {
		p["purpose"] = params.Purpose
	}
	if params.ValidityMinutes > 0 {
		p["validity"] = params.ValidityMinutes
	}
	if params.LookAndFeelProfile != "" {
		p["lookAndFeelProfile"] = params.LookAndFeelProfile
	}
	if len(params.PaymentMeans) > 0 {
		p["pm"] = params.PaymentMeans
	}
	if len(params.InvoiceItems) > 0 {
		basket := make([]map[string]any, len(params.InvoiceItems))
		for i, item := range params.InvoiceItems {
			basket[i] = map[string]any{
				"name":     map[string]any{"1": item.Name},
				"quantity": item.Quantity,
				"amount":   item.Amount,
			}
		}
		p["basket"] = basket
	}

	body, err := c.doRequest(ctx, "POST", "Gateway/", p)
	if err != nil {
		return nil, err
	}

	var resp GatewayResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("payrexx: failed to parse response: %w", err)
	}

	if resp.Status != "success" || len(resp.Data) == 0 {
		return nil, fmt.Errorf("payrexx: failed to create gateway: %s", resp.Message)
	}

	return &resp.Data[0], nil
}

func (c *Client) GetGateway(ctx context.Context, id int) (*Gateway, error) {
	body, err := c.doRequest(ctx, "GET", fmt.Sprintf("Gateway/%d/", id), nil)
	if err != nil {
		return nil, err
	}

	var resp GatewayResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("payrexx: failed to parse response: %w", err)
	}

	if resp.Status != "success" || len(resp.Data) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, resp.Message)
	}

	return &resp.Data[0], nil
}

func (c *Client) DeleteGateway(ctx context.Context, id int) error {
	_, err := c.doRequest(ctx, "DELETE", fmt.Sprintf("Gateway/%d/", id), nil)
	return err
}

const (
	GatewayStatusWaiting   = "waiting"
	GatewayStatusConfirmed = "confirmed"
	GatewayStatusCancelled = "cancelled"
	GatewayStatusDeclined  = "declined"
	GatewayStatusError     = "error"
	GatewayStatusExpired   = "expired"
)
