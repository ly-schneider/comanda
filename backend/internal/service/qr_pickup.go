package service

import (
	"context"
	"fmt"
	"time"

	"backend/internal/qrsign"
)

// signPickupToken signs a pickup token over an already-computed redeemable line
// set, using the tenant's Ed25519 key. Both order creation and refund re-issue
// funnel their (differently derived) line sets through here so the signing and
// payload shape stay identical.
func signPickupToken(ctx context.Context, qrKeys QRKeyService, tenantID, orderID string, lines []qrsign.Line) (string, error) {
	if qrKeys == nil {
		return "", fmt.Errorf("qr signing not configured")
	}
	priv, _, err := qrKeys.EnsureTenantKey(ctx)
	if err != nil {
		return "", fmt.Errorf("qr sign: ensure tenant key: %w", err)
	}
	token, err := qrsign.Sign(priv, qrsign.Payload{
		Version:  qrsign.Version,
		TenantID: tenantID,
		OrderID:  orderID,
		IssuedAt: time.Now().Unix(),
		Lines:    lines,
	})
	if err != nil {
		return "", fmt.Errorf("qr sign: sign payload: %w", err)
	}
	return token, nil
}
