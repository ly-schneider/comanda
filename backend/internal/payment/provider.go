// Package payment defines the provider-agnostic payment abstraction and the
// per-tenant provider factory. Payrexx is the first implementation.
package payment

import (
	"context"

	"backend/internal/payrexx"
)

type CheckoutRequest struct {
	AmountCents        int
	Currency           string
	ReferenceID        string
	SuccessRedirectURL string
	FailedRedirectURL  string
	CancelRedirectURL  string
	PaymentMeans       []string
	LineItems          []LineItem
	CustomerEmail      string
	Purpose            string
	ValidityMinutes    int
}

type LineItem struct {
	Name     string
	Quantity int
	Amount   int
}

type CheckoutResult struct {
	GatewayID   int
	Link        string
	Status      string
	ReferenceID string
	// Settlement is populated by GetCheckout only: what the provider says about
	// the money behind this checkout, so a payment can be confirmed by pulling
	// when the webhook never arrives.
	Settlement CheckoutSettlement
}

// CheckoutSettlement mirrors the terminal facts a success webhook would carry.
type CheckoutSettlement struct {
	Paid          bool
	Failed        bool
	TransactionID int
	ContactEmail  string
}

type WebhookResult struct {
	ReferenceID   string
	TransactionID int
	GatewayID     int
	Status        string
	ContactEmail  string
	// AmountCents is the original charge; RefundedAmountCents is the cumulative
	// refunded amount. Reconcile against the latter.
	AmountCents         int
	RefundedAmountCents int
	IsSuccess           bool
	IsFailure           bool
	IsRefund            bool
	IsFullRefund        bool
}

// Provider is built from a single tenant's decrypted credentials, so an
// instance is inherently scoped to one tenant.
type Provider interface {
	CreateCheckout(ctx context.Context, req CheckoutRequest) (*CheckoutResult, error)
	GetCheckout(ctx context.Context, gatewayID int) (*CheckoutResult, error)
	Refund(ctx context.Context, transactionID int, amountCents int) error
	VerifyWebhook(body []byte, signature string) bool
	ParseWebhook(body []byte) (*WebhookResult, error)
}

type payrexxProvider struct {
	client        *payrexx.Client
	webhookSecret string
}

func NewPayrexxProvider(instance, apiSecret, webhookSecret string) Provider {
	return &payrexxProvider{
		client:        payrexx.NewClient(instance, apiSecret),
		webhookSecret: webhookSecret,
	}
}

func (p *payrexxProvider) CreateCheckout(ctx context.Context, req CheckoutRequest) (*CheckoutResult, error) {
	items := make([]payrexx.InvoiceItem, len(req.LineItems))
	for i, li := range req.LineItems {
		items[i] = payrexx.InvoiceItem{Name: li.Name, Quantity: li.Quantity, Amount: li.Amount}
	}
	gw, err := p.client.CreateGateway(ctx, payrexx.CreateGatewayParams{
		Amount:             req.AmountCents,
		Currency:           req.Currency,
		ReferenceID:        req.ReferenceID,
		SuccessRedirectURL: req.SuccessRedirectURL,
		FailedRedirectURL:  req.FailedRedirectURL,
		CancelRedirectURL:  req.CancelRedirectURL,
		PaymentMeans:       req.PaymentMeans,
		InvoiceItems:       items,
		CustomerEmail:      req.CustomerEmail,
		Purpose:            req.Purpose,
		ValidityMinutes:    req.ValidityMinutes,
	})
	if err != nil {
		return nil, err
	}
	return &CheckoutResult{GatewayID: gw.ID, Link: gw.Link, Status: gw.Status, ReferenceID: gw.ReferenceID}, nil
}

func (p *payrexxProvider) GetCheckout(ctx context.Context, gatewayID int) (*CheckoutResult, error) {
	gw, err := p.client.GetGateway(ctx, gatewayID)
	if err != nil {
		return nil, err
	}
	st := gw.Settle()
	return &CheckoutResult{
		GatewayID:   gw.ID,
		Link:        gw.Link,
		Status:      gw.Status,
		ReferenceID: gw.ReferenceID,
		Settlement: CheckoutSettlement{
			Paid:          st.Paid,
			Failed:        st.Failed,
			TransactionID: st.TransactionID,
			ContactEmail:  st.ContactEmail,
		},
	}, nil
}

func (p *payrexxProvider) Refund(ctx context.Context, transactionID int, amountCents int) error {
	return p.client.RefundTransaction(ctx, transactionID, amountCents)
}

func (p *payrexxProvider) VerifyWebhook(body []byte, signature string) bool {
	return payrexx.VerifyWebhookSignature(body, signature, p.webhookSecret)
}

func (p *payrexxProvider) ParseWebhook(body []byte) (*WebhookResult, error) {
	event, err := payrexx.ParseWebhookEvent(body)
	if err != nil {
		return nil, err
	}
	txn := &event.Transaction
	out := &WebhookResult{
		ReferenceID:         txn.ReferenceID,
		TransactionID:       txn.ID,
		Status:              txn.Status,
		ContactEmail:        txn.Contact.Email,
		AmountCents:         txn.Amount,
		RefundedAmountCents: txn.RefundedAmount,
		IsSuccess:           txn.IsSuccessStatus(),
		IsFailure:           txn.IsFailedStatus(),
		IsRefund:            txn.IsRefundStatus(),
		IsFullRefund:        txn.IsFullRefundStatus(),
	}
	if txn.Invoice.PaymentRequestID != nil {
		out.GatewayID = *txn.Invoice.PaymentRequestID
	}
	return out, nil
}
