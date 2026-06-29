package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/refund"
	"backend/internal/response"
	"backend/internal/service"
)

// ── Actor resolution ────────────────────────────────────────────────────────

// posRefundActor builds the operator actor for a POS-device request.
func posRefundActor(ctx context.Context) (service.RefundActor, bool) {
	deviceID, ok := auth.GetDeviceID(ctx)
	if !ok {
		return service.RefundActor{}, false
	}
	return service.RefundActor{
		Type:     refund.ActorTypeOperator,
		ID:       deviceID,
		DeviceID: &deviceID,
	}, true
}

// adminRefundActor builds the admin actor for a session-authenticated request.
func adminRefundActor(ctx context.Context) (service.RefundActor, bool) {
	userID, ok := auth.GetUserID(ctx)
	if !ok {
		return service.RefundActor{}, false
	}
	return service.RefundActor{
		Type: refund.ActorTypeAdmin,
		ID:   userID,
	}, true
}

// ── Shared handler bodies (one per logical action, reused by both surfaces) ──

func (h *Handlers) doRefundLookup(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}
	orders, err := h.refunds.LookupForPOS(r.Context(), q)
	if err != nil {
		writeRefundError(w, err)
		return
	}
	items := make([]generated.Order, 0, len(orders))
	for _, o := range orders {
		items = append(items, toAPIOrder(o))
	}
	response.WriteJSON(w, http.StatusOK, generated.OrderLookupResult{Items: items})
}

func (h *Handlers) doRefundView(w http.ResponseWriter, r *http.Request, orderID string) {
	view, err := h.refunds.GetRefundView(r.Context(), orderID)
	if err != nil {
		writeRefundError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIRefundView(view))
}

func (h *Handlers) doLineRefund(w http.ResponseWriter, r *http.Request, orderID string, actor service.RefundActor) {
	var body generated.LineRefundRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if len(body.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "no_lines", "At least one refund line is required")
		return
	}
	lines := make([]service.RefundLineQty, 0, len(body.Lines))
	for _, l := range body.Lines {
		lines = append(lines, service.RefundLineQty{OrderLineID: l.OrderLineId, Quantity: l.Quantity})
	}
	ref, err := h.refunds.RefundLines(r.Context(), service.RefundLinesInput{
		OrderID: orderID,
		Lines:   lines,
		Reason:  body.Reason,
		Actor:   actor,
	})
	if err != nil {
		writeRefundError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIRefund(ref))
}

func (h *Handlers) doCancelOrder(w http.ResponseWriter, r *http.Request, orderID string, actor service.RefundActor) {
	var reason *string
	var body generated.CancelOrderRequest
	// Body is optional; ignore a missing/empty payload.
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
		reason = body.Reason
	}
	ref, err := h.refunds.CancelOrder(r.Context(), service.CancelOrderInput{
		OrderID: orderID,
		Reason:  reason,
		Actor:   actor,
	})
	if err != nil {
		writeRefundError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIRefund(ref))
}

// ── POS device surface (operator) ───────────────────────────────────────────

// (GET /pos/orders/lookup)
func (h *Handlers) LookupPosOrders(w http.ResponseWriter, r *http.Request) {
	h.doRefundLookup(w, r)
}

// (GET /pos/orders/{orderId}/refunds)
func (h *Handlers) GetPosRefundView(w http.ResponseWriter, r *http.Request, orderId string) {
	h.doRefundView(w, r, orderId)
}

// (POST /pos/orders/{orderId}/refunds)
func (h *Handlers) CreatePosLineRefund(w http.ResponseWriter, r *http.Request, orderId string) {
	actor, ok := posRefundActor(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Device authentication required")
		return
	}
	h.doLineRefund(w, r, orderId, actor)
}

// (POST /pos/orders/{orderId}/cancel)
func (h *Handlers) CancelPosOrder(w http.ResponseWriter, r *http.Request, orderId string) {
	actor, ok := posRefundActor(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Device authentication required")
		return
	}
	h.doCancelOrder(w, r, orderId, actor)
}

// ── Authenticated admin surface ─────────────────────────────────────────────

// (GET /orders/lookup)
func (h *Handlers) LookupOrders(w http.ResponseWriter, r *http.Request) {
	h.doRefundLookup(w, r)
}

// (GET /orders/{orderId}/refunds)
func (h *Handlers) GetRefundView(w http.ResponseWriter, r *http.Request, orderId string) {
	h.doRefundView(w, r, orderId)
}

// (POST /orders/{orderId}/refunds)
func (h *Handlers) CreateLineRefund(w http.ResponseWriter, r *http.Request, orderId string) {
	actor, ok := adminRefundActor(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	h.doLineRefund(w, r, orderId, actor)
}

// (POST /orders/{orderId}/cancel)
func (h *Handlers) CancelOrder(w http.ResponseWriter, r *http.Request, orderId string) {
	actor, ok := adminRefundActor(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	h.doCancelOrder(w, r, orderId, actor)
}

// CreateArbitraryRefund issues a discretionary, line-independent refund. The
// app-admin gate is enforced by the router group (TierAppAdmin); POS operators
// have no route to this action.
// (POST /orders/{orderId}/refunds/arbitrary)
func (h *Handlers) CreateArbitraryRefund(w http.ResponseWriter, r *http.Request, orderId string) {
	actor, ok := adminRefundActor(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	var body generated.ArbitraryRefundRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	ref, err := h.refunds.RefundArbitrary(r.Context(), service.RefundArbitraryInput{
		OrderID:     orderId,
		AmountCents: body.AmountCents,
		Reason:      body.Reason,
		Actor:       actor,
	})
	if err != nil {
		writeRefundError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIRefund(ref))
}

// ── Mappers ─────────────────────────────────────────────────────────────────

func toAPIRefund(e *ent.Refund) generated.Refund {
	r := generated.Refund{
		Id:          e.ID,
		OrderId:     e.OrderID,
		AmountCents: e.AmountCents,
		Kind:        generated.RefundKind(e.Kind),
		Method:      generated.RefundMethod(e.Method),
		Reason:      e.Reason,
		ActorType:   generated.RefundActorType(e.ActorType),
		ActorId:     e.ActorID,
		DeviceId:    e.DeviceID,
		Status:      generated.RefundStatus(e.Status),
		CreatedAt:   e.CreatedAt,
	}
	if lines, err := e.Edges.RefundLinesOrErr(); err == nil && len(lines) > 0 {
		apiLines := make([]generated.RefundLine, 0, len(lines))
		for _, l := range lines {
			apiLines = append(apiLines, toAPIRefundLine(l))
		}
		r.Lines = &apiLines
	}
	return r
}

func toAPIRefundLine(e *ent.RefundLine) generated.RefundLine {
	return generated.RefundLine{
		Id:          e.ID,
		OrderLineId: e.OrderLineID,
		Quantity:    e.Quantity,
		AmountCents: e.AmountCents,
	}
}

func toAPIRefundView(v *service.RefundView) generated.RefundView {
	lines := make([]generated.RefundViewLine, 0, len(v.Lines))
	for _, l := range v.Lines {
		lines = append(lines, generated.RefundViewLine{
			OrderLineId:        l.Line.ID,
			ProductId:          l.Line.ProductID,
			Title:              l.Line.Title,
			UnitPriceCents:     l.Line.UnitPriceCents,
			BoughtQuantity:     l.BoughtQuantity,
			RefundedQuantity:   l.RefundedQuantity,
			RefundableQuantity: l.RefundableQty,
			Redeemed:           l.Redeemed,
		})
	}
	refunds := make([]generated.Refund, 0, len(v.Refunds))
	for _, rf := range v.Refunds {
		refunds = append(refunds, toAPIRefund(rf))
	}
	return generated.RefundView{
		Order:                  toAPIOrder(v.Order),
		Lines:                  lines,
		RefundableBalanceCents: v.RefundableBalanceCents,
		Refunds:                refunds,
	}
}
