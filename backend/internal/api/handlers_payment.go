package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

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

	ctx, finalize, err := h.beginTenantTx(r.Context(), tenantID)
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

	// A verified signature proves the stored webhook secret matches Payrexx's;
	// record it (best effort) before branching on status so the setup UI can
	// confirm delivery even for non-success ("ignored") events.
	if err := h.payments.RecordPayrexxWebhookReceived(ctx); err != nil {
		h.logger.Warn("payrexx webhook: failed to record receipt",
			zap.String("tenantId", tenantID), zap.Error(err))
	}

	result, err := prov.ParseWebhook(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid webhook payload")
		return
	}

	// Refund webhooks carry IsRefund=true, IsSuccess=false, so they must be
	// handled before the IsSuccess gate below (which would otherwise drop them as
	// "ignored"). Reconcile is defensive + idempotent: it only records a refund
	// the provider performed that our ledger doesn't already reflect (e.g. a
	// dashboard-issued refund), so on failure we still ack 200 to avoid a retry
	// storm; the deferred rollback keeps a failed reconcile from half-writing.
	if result.IsRefund {
		if err := h.refunds.ReconcileFromWebhook(ctx, result); err != nil {
			h.logger.Error("payrexx webhook: refund reconcile failed",
				zap.Error(err),
				zap.String("tenantId", tenantID),
				zap.String("referenceId", result.ReferenceID),
			)
			response.WriteJSON(w, http.StatusOK, map[string]string{"status": "refund_reconcile_failed"})
			return
		}
		response.WriteJSON(w, http.StatusOK, map[string]string{"status": "refund_reconciled"})
		finalize(true)
		committed = true
		return
	}

	if !result.IsSuccess {
		// A failed/expired payment is the terminal signal that this order won't be
		// paid: release its inventory hold now instead of waiting for the backstop
		// sweep. Best-effort — still ack 200 so Payrexx doesn't retry.
		if nanoid.Valid(result.ReferenceID) {
			if err := h.payments.CleanupPendingOrderByID(ctx, result.ReferenceID); err != nil {
				h.logger.Warn("payrexx webhook: release on payment failure failed",
					zap.String("orderId", result.ReferenceID),
					zap.String("tenantId", tenantID),
					zap.Error(err),
				)
			}
		}
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

// beginTenantTx opens a short tenant-scoped tx mirroring the request tenant-tx
// middleware, so RLS behaves identically off the request path (webhook, SSE
// snapshot). finalize(true) commits, finalize(false) rolls back.
func (h *Handlers) beginTenantTx(ctx context.Context, tenantID string) (context.Context, func(ok bool), error) {
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
	ctx = repository.ContextWithRequestTx(ctx, tx)
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

func payrexxStatusResponse(status service.PayrexxStatus) map[string]any {
	resp := map[string]any{"state": string(status.State)}
	if status.Instance != "" {
		resp["instance"] = status.Instance
	}
	if status.WebhookURL != "" {
		resp["webhookUrl"] = status.WebhookURL
	}
	if status.LastWebhookAt != nil {
		resp["lastWebhookAt"] = status.LastWebhookAt.UTC().Format(time.RFC3339)
	}
	return resp
}

// (GET /settings/payrexx)
func (h *Handlers) GetPayrexxStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.payments.GetPayrexxStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read Payrexx status")
		return
	}
	response.WriteJSON(w, http.StatusOK, payrexxStatusResponse(status))
}

// (PUT /settings/payrexx/api)
func (h *Handlers) ConnectPayrexxApi(w http.ResponseWriter, r *http.Request) {
	var body generated.ConnectPayrexxApiJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	status, err := h.payments.ConnectPayrexxAPI(r.Context(), body.Instance, body.ApiSecret)
	if err != nil {
		h.logger.Warn("connect payrexx api failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, "configure_failed", "Could not validate Payrexx credentials. Check the instance name and API key.")
		return
	}

	response.WriteJSON(w, http.StatusOK, payrexxStatusResponse(status))
}

// (PUT /settings/payrexx/webhook)
func (h *Handlers) SetPayrexxWebhookSecret(w http.ResponseWriter, r *http.Request) {
	var body generated.SetPayrexxWebhookSecretJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	status, err := h.payments.SetPayrexxWebhookSecret(r.Context(), body.WebhookSecret)
	if err != nil {
		h.logger.Warn("set payrexx webhook secret failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, "configure_failed", "Could not store the webhook secret. Connect the Payrexx API first.")
		return
	}

	response.WriteJSON(w, http.StatusOK, payrexxStatusResponse(status))
}

// (POST /settings/payrexx/test)
func (h *Handlers) TestPayrexxConnection(w http.ResponseWriter, r *http.Request) {
	if err := h.payments.TestPayrexxConnection(r.Context()); err != nil {
		h.logger.Warn("test payrexx connection failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, "test_failed", "Could not reach Payrexx with the stored credentials.")
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GetPayment returns a payment by ID.
// (GET /payments/{paymentId})
func (h *Handlers) GetPayment(w http.ResponseWriter, r *http.Request, paymentId string) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "Payment lookup by ID will be available after service refactoring")
}
