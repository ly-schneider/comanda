package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/settings"
	"backend/internal/repository"
	"backend/internal/response"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

// HandlePayrexxWebhook resolves the opaque {tenantToken} to a tenant via the
// privileged client (the request is unauthenticated and has no tenant-bearing
// Host), then runs everything under that tenant's RLS GUC. The HMAC is verified
// with the resolved tenant's own secret, so a webhook can never be
// cross-attributed; wrong token -> 404, bad/foreign signature -> 401.
func (h *Handlers) HandlePayrexxWebhook(w http.ResponseWriter, r *http.Request, tenantToken string) {
	if tenantToken == "" {
		writeError(w, http.StatusNotFound, "unknown_tenant", "Unknown webhook tenant")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Failed to read webhook payload")
		return
	}

	tenantID, err := h.resolveWebhookTenant(r.Context(), tenantToken)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "unknown_tenant", "Unknown webhook tenant")
			return
		}
		h.logger.Error("payrexx webhook: tenant resolution failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "resolution_error", "Failed to resolve tenant")
		return
	}

	ctx, finalize, err := h.beginWebhookTenantTx(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("payrexx webhook: begin tenant tx", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to open tenant context")
		return
	}
	committed := false
	defer func() {
		if !committed {
			finalize(false)
		}
	}()

	prov, err := h.providers.For(ctx)
	if err != nil {
		h.logger.Warn("payrexx webhook: tenant has no provider configured",
			zap.String("tenantId", tenantID), zap.Error(err))
		writeError(w, http.StatusUnauthorized, "not_configured", "Payment provider not configured")
		return
	}

	signature := r.Header.Get("X-Webhook-Signature")
	if !prov.VerifyWebhook(body, signature) {
		h.logger.Warn("payrexx webhook: invalid signature", zap.String("tenantId", tenantID))
		writeError(w, http.StatusUnauthorized, "invalid_signature", "Invalid webhook signature")
		return
	}

	result, err := prov.ParseWebhook(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid webhook payload")
		return
	}

	if !result.IsSuccess {
		response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		finalize(true)
		committed = true
		return
	}

	if result.ReferenceID == "" {
		h.logger.Warn("payrexx webhook: missing referenceId")
		writeError(w, http.StatusBadRequest, "invalid_payload", "Missing referenceId")
		return
	}

	orderID := result.ReferenceID
	if !nanoid.Valid(orderID) {
		h.logger.Warn("payrexx webhook: invalid referenceId", zap.String("referenceId", result.ReferenceID))
		writeError(w, http.StatusBadRequest, "invalid_payload", "Invalid referenceId")
		return
	}

	var contactEmail *string
	if result.ContactEmail != "" {
		contactEmail = &result.ContactEmail
	}

	if err := h.payments.MarkOrderPaidByPayrexx(ctx, orderID, result.GatewayID, result.TransactionID, contactEmail); err != nil {
		h.logger.Error("payrexx webhook: failed to mark order paid",
			zap.Error(err),
			zap.String("orderId", orderID),
		)
		writeError(w, http.StatusInternalServerError, "processing_error", "Failed to process payment")
		return
	}

	h.logger.Info("payrexx webhook: order marked as paid",
		zap.String("tenantId", tenantID),
		zap.String("orderId", orderID),
		zap.Int("gatewayId", result.GatewayID),
		zap.Int("transactionId", result.TransactionID),
	)

	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	finalize(true)
	committed = true
}

// resolveWebhookTenant uses the privileged (RLS-bypassing) client because it
// runs before any tenant context exists, like the host->tenant middleware.
func (h *Handlers) resolveWebhookTenant(ctx context.Context, token string) (string, error) {
	row, err := h.privileged.Settings.Query().
		Where(settings.PaymentWebhookTokenEQ(token)).
		Only(ctx)
	if err != nil {
		return "", err
	}
	return row.TenantID, nil
}

// beginWebhookTenantTx mirrors the request tenant-tx middleware so RLS behaves
// identically on the webhook path.
func (h *Handlers) beginWebhookTenantTx(ctx context.Context, tenantID string) (context.Context, func(ok bool), error) {
	conn, err := h.appDB.DB.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"SELECT set_config('app.tenant_id', $1, true), set_config('app.is_platform_admin', 'false', true)",
		tenantID,
	); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, nil, err
	}

	txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
	enttenant.Register(txClient)

	ctx = repository.ContextWithClient(ctx, txClient)
	ctx = repository.ContextWithTenantID(ctx, tenantID)
	ctx = enttenant.WithTenant(ctx, tenantID)

	finalize := func(ok bool) {
		if ok {
			_ = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		_ = conn.Close()
	}
	return ctx, finalize, nil
}

// (GET /settings/payrexx)
func (h *Handlers) GetPayrexxStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.payments.GetPayrexxStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read Payrexx status")
		return
	}
	resp := map[string]any{"configured": status.Configured}
	if status.Configured {
		resp["instance"] = status.Instance
		resp["webhookUrl"] = status.WebhookURL
	}
	response.WriteJSON(w, http.StatusOK, resp)
}

// (PUT /settings/payrexx)
func (h *Handlers) ConfigurePayrexx(w http.ResponseWriter, r *http.Request) {
	var body generated.ConfigurePayrexxJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	webhookURL, err := h.payments.ConfigurePayrexx(r.Context(), service.ConfigurePayrexxInput{
		Instance:      body.Instance,
		APISecret:     body.ApiSecret,
		WebhookSecret: body.WebhookSecret,
	})
	if err != nil {
		h.logger.Warn("configure payrexx failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, "configure_failed", "Could not validate Payrexx credentials. Check the instance name and API secret.")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"webhookUrl": webhookURL,
	})
}

// GetPayment returns a payment by ID.
// (GET /payments/{paymentId})
func (h *Handlers) GetPayment(w http.ResponseWriter, r *http.Request, paymentId string) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "Payment lookup by ID will be available after service refactoring")
}
