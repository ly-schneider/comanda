package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/plugin"
	"backend/internal/repository"
	"backend/internal/response"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

const (
	idempotencyScopeOrder   = "order_create"
	idempotencyScopePayment = "order_payment"
	idempotencyTTL          = 24 * time.Hour
)

// ListOrders returns orders with optional filtering.
// (GET /orders)
func (h *Handlers) ListOrders(w http.ResponseWriter, r *http.Request, params generated.ListOrdersParams) {
	ctx := r.Context()

	isAdmin, err := auth.HasTenantTier(ctx, auth.TierOps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
		return
	}
	scopeMine := params.Scope != nil && *params.Scope == generated.Mine

	if !isAdmin || scopeMine {
		uid, ok := auth.GetUserID(ctx)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		rows, _, err := h.orders.ListByCustomerID(ctx, uid)
		if err != nil {
			writeEntError(w, err)
			return
		}
		items := make([]generated.Order, 0, len(rows))
		for _, o := range rows {
			items = append(items, toAPIOrder(o))
		}
		response.WriteJSON(w, http.StatusOK, generated.OrderList{Items: items})
		return
	}

	listParams := service.OrderListParams{}
	if params.Status != nil {
		s := order.Status(*params.Status)
		listParams.Status = &s
	}
	if params.DateFrom != nil {
		fromStr := params.DateFrom.Format("2006-01-02T15:04:05Z07:00")
		listParams.From = &fromStr
	}
	if params.DateTo != nil {
		toStr := params.DateTo.Format("2006-01-02T15:04:05Z07:00")
		listParams.To = &toStr
	}
	if params.Q != nil {
		q := strings.TrimSpace(*params.Q)
		if q != "" {
			listParams.Query = &q
		}
	}
	if params.Include != nil && *params.Include == generated.Lines {
		listParams.IncludeLines = true
	}

	orders, _, err := h.orders.ListAdmin(ctx, listParams)
	if err != nil {
		writeEntError(w, err)
		return
	}

	items := make([]generated.Order, 0, len(orders))
	for _, o := range orders {
		items = append(items, toAPIOrder(o))
	}
	response.WriteJSON(w, http.StatusOK, generated.OrderList{Items: items})
}

// CreateOrder creates a new order from cart items.
// (POST /orders)
func (h *Handlers) CreateOrder(w http.ResponseWriter, r *http.Request, params generated.CreateOrderParams) {
	ctx := r.Context()

	var idempotencyKey string
	if params.IdempotencyKey != nil {
		idempotencyKey = *params.IdempotencyKey
	}
	var claimedID *string
	if idempotencyKey != "" {
		claimed, existed, err := h.idempotency.Claim(ctx, idempotencyScopeOrder, idempotencyKey, idempotencyTTL)
		if err != nil {
			h.logger.Warn("idempotency claim failed", zap.Error(err))
		} else if existed {
			respMap, _ := repository.GetResponseMap(claimed)
			if respMap != nil {
				response.WriteJSON(w, http.StatusOK, respMap)
				return
			}
			writeError(w, http.StatusConflict, "in_progress", "Request with this idempotency key is being processed")
			return
		} else {
			id := claimed.ID
			claimedID = &id
		}
	}

	var body generated.OrderCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if claimedID != nil {
			_ = h.idempotency.Discard(ctx, *claimedID)
		}
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(body.Items) == 0 {
		if claimedID != nil {
			_ = h.idempotency.Discard(ctx, *claimedID)
		}
		writeError(w, http.StatusBadRequest, "no_items", "At least one item is required")
		return
	}

	checkoutItems := make([]service.CheckoutItemInput, 0, len(body.Items))
	for _, item := range body.Items {
		ci := service.CheckoutItemInput{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
			VariantID: item.VariantId,
		}
		if item.BundleSelections != nil {
			ci.Configuration = make(map[string]string, len(*item.BundleSelections))
			for _, sel := range *item.BundleSelections {
				ci.Configuration[sel.SlotId] = sel.ProductId
				if sel.VariantId != nil && *sel.VariantId != "" {
					if ci.ConfigurationVariants == nil {
						ci.ConfigurationVariants = make(map[string]string, len(*item.BundleSelections))
					}
					ci.ConfigurationVariants[sel.SlotId] = *sel.VariantId
				}
			}
		}
		checkoutItems = append(checkoutItems, ci)
	}

	var customerEmail *string
	if body.ContactEmail != nil {
		email := string(*body.ContactEmail)
		customerEmail = &email
	}

	origin := order.OriginShop
	if _, isDevice := auth.GetDeviceID(ctx); isDevice {
		origin = order.OriginPos
	}

	input := service.CreateCheckoutInput{
		Items:         checkoutItems,
		CustomerEmail: customerEmail,
		Origin:        origin,
	}

	var userID *string
	if origin != order.OriginPos {
		if uid, ok := auth.GetUserID(ctx); ok {
			userID = &uid
		}
	}

	prep, err := h.payments.PrepareAndCreateOrder(ctx, input, userID, nil)
	if err != nil {
		if claimedID != nil {
			_ = h.idempotency.Discard(ctx, *claimedID)
		}
		var soldOut *service.InsufficientInventoryError
		if errors.As(err, &soldOut) {
			writeOutOfStockError(w, soldOut)
			return
		}
		var unavailable *service.ProductUnavailableError
		if errors.As(err, &unavailable) {
			writeProductUnavailableError(w, unavailable)
			return
		}
		writeError(w, http.StatusBadRequest, "order_failed", err.Error())
		return
	}

	apiOrder := toAPIOrder(prep.Order)

	if claimedID != nil {
		respMap := map[string]any{
			"id":         apiOrder.Id,
			"status":     string(apiOrder.Status),
			"totalCents": apiOrder.TotalCents,
			"createdAt":  apiOrder.CreatedAt.Format(time.RFC3339),
			"origin":     string(apiOrder.Origin),
		}
		if err := h.idempotency.FillResponse(ctx, *claimedID, respMap); err != nil {
			h.logger.Warn("failed to fill idempotent order response", zap.Error(err))
		}
	}

	response.WriteJSON(w, http.StatusCreated, apiOrder)
}

// GetOrder returns a single order by ID with lines, payments, and redemptions.
// (GET /orders/{orderId})
func (h *Handlers) GetOrder(w http.ResponseWriter, r *http.Request, orderId string) {
	ctx := r.Context()
	o, err := h.orders.GetByIDWithRelations(ctx, orderId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	apiOrder := toAPIOrder(o)
	// This route is public and stations verify the token offline, so an unpaid
	// order's pickup token must not leave the server.
	if o.Status == order.StatusPending {
		apiOrder.QrPayload = nil
	}
	if orderReflectsRefunds(o.Status) {
		if err := h.annotateOrderRefunds(ctx, &apiOrder); err != nil {
			h.logger.Warn("load refund summary", zap.String("order_id", o.ID), zap.Error(err))
		}
	}
	response.WriteJSON(w, http.StatusOK, apiOrder)
}

// orderReflectsRefunds reports whether an order's status implies money was
// refunded, so the refund summary is only loaded for those orders.
func orderReflectsRefunds(s order.Status) bool {
	return s == order.StatusRefunded || s == order.StatusPartiallyRefunded || s == order.StatusCancelled
}

func (h *Handlers) annotateOrderRefunds(ctx context.Context, o *generated.Order) error {
	summary, err := h.refunds.RefundedSummary(ctx, o.Id)
	if err != nil {
		return err
	}
	if summary.TotalCents > 0 {
		o.RefundedTotalCents = ptr(summary.TotalCents)
	}
	if o.Lines != nil {
		annotateLineRefunds(*o.Lines, summary.ByLine)
	}
	return nil
}

func annotateLineRefunds(lines []generated.OrderLine, byLine map[string]int) {
	for i := range lines {
		if q := byLine[lines[i].Id]; q > 0 {
			lines[i].RefundedQuantity = ptr(q)
		}
		if lines[i].ChildLines != nil {
			annotateLineRefunds(*lines[i].ChildLines, byLine)
		}
	}
}

// UpdateOrderStatus updates the status of an order.
// (PATCH /orders/{orderId})
func (h *Handlers) UpdateOrderStatus(w http.ResponseWriter, r *http.Request, orderId string) {
	var body generated.OrderStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ctx := r.Context()
	id := orderId

	// After Task 6, UpdateStatus takes order.Status from the ent package.
	if err := h.orders.UpdateStatus(ctx, id, order.Status(body.Status)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_transition", err.Error())
		return
	}

	updated, err := h.orders.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIOrder(updated))
}

// DiscardPendingOrder deletes an unpaid pending POS order and releases its hold
// when the cashier abandons a checkout before payment. Idempotent: a missing or
// already-settled order returns 204. Restricted to POS orders — shop pending
// orders are reclaimed by the payment-failure webhook / backstop sweeper.
func (h *Handlers) DiscardPendingOrder(w http.ResponseWriter, r *http.Request, orderId string) {
	ctx := r.Context()

	if !nanoid.Valid(orderId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid order id")
		return
	}

	ord, err := h.orders.GetByID(ctx, orderId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, repository.ErrNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeEntError(w, err)
		return
	}
	if ord.Origin != order.OriginPos {
		writeError(w, http.StatusForbidden, "forbidden", "Only POS orders can be discarded.")
		return
	}
	if err := h.payments.CleanupPendingOrderByID(ctx, orderId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetOrderPayment returns the payment details for an order.
// (GET /orders/{orderId}/payment)
func (h *Handlers) GetOrderPayment(w http.ResponseWriter, r *http.Request, orderId string) {
	ctx := r.Context()

	o, err := h.orders.GetByID(ctx, orderId)
	if err != nil {
		writeEntError(w, err)
		return
	}

	// The customer waiting on the return page polls this, so it is the earliest
	// place a lost webhook can be caught. Best effort: a slow or failing provider
	// must not break the status read, and the reconciler covers what's missed.
	if o.Status == order.StatusPending && o.PayrexxGatewayID != nil {
		if res, err := h.payments.TryReconcilePendingOrder(ctx, orderId); err != nil {
			h.logger.Warn("payment status: reconcile failed",
				zap.String("orderId", orderId), zap.Error(err))
		} else if res.Settled {
			if refreshed, err := h.orders.GetByID(ctx, orderId); err == nil {
				o = refreshed
			}
		}
	}

	if o.PayrexxGatewayID == nil {
		if o.Status == order.StatusPaid {
			response.WriteJSON(w, http.StatusOK, map[string]any{
				"orderId": o.ID,
				"status":  "paid",
			})
			return
		}
		writeError(w, http.StatusNotFound, "no_payment", "No payment gateway associated with this order")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{
		"orderId": o.ID,
		"status":  string(o.Status),
	})
}

// CreateOrderPayment creates a payment for an order.
// (POST /orders/{orderId}/payment)
func (h *Handlers) CreateOrderPayment(w http.ResponseWriter, r *http.Request, orderId string, params generated.CreateOrderPaymentParams) {
	ctx := r.Context()
	id := orderId

	var idempotencyKey string
	if params.IdempotencyKey != nil {
		idempotencyKey = *params.IdempotencyKey
	}
	var claimedID *string
	if idempotencyKey != "" {
		scopeKey := idempotencyScopePayment + ":" + id
		claimed, existed, err := h.idempotency.Claim(ctx, scopeKey, idempotencyKey, idempotencyTTL)
		if err != nil {
			h.logger.Warn("idempotency claim failed", zap.Error(err))
		} else if existed {
			respMap, _ := repository.GetResponseMap(claimed)
			if respMap != nil {
				response.WriteJSON(w, http.StatusOK, respMap)
				return
			}
			writeError(w, http.StatusConflict, "in_progress", "Request with this idempotency key is being processed")
			return
		} else {
			cid := claimed.ID
			claimedID = &cid
		}
	}

	paymentSucceeded := false
	defer func() {
		if claimedID != nil && !paymentSucceeded {
			_ = h.idempotency.Discard(ctx, *claimedID)
		}
	}()

	var body generated.PaymentCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	cachePaymentResponse := func(resp map[string]any) {
		paymentSucceeded = true
		if claimedID != nil {
			if err := h.idempotency.FillResponse(ctx, *claimedID, resp); err != nil {
				h.logger.Warn("failed to fill idempotent payment response", zap.Error(err))
			}
		}
	}

	twintPOS := body.Channel != nil && *body.Channel == generated.PaymentChannelPos
	supportsPlugin := body.Method == generated.Cash ||
		body.Method == generated.Card ||
		(body.Method == generated.Twint && twintPOS) ||
		body.Method == generated.GratisPlugin

	var (
		pluginHook  plugin.PaymentHook
		pluginData  json.RawMessage
		pluginOrder *ent.Order
		pluginKey   *string
	)
	if body.Plugin != nil {
		if !supportsPlugin {
			writeError(w, http.StatusBadRequest, "plugin_not_supported", "This payment method does not support a plugin redemption")
			return
		}
		hook, err := h.plugins.PaymentHook(ctx, body.Plugin.Key)
		if err != nil {
			h.logger.Warn("plugin payment rejected", zap.String("plugin", body.Plugin.Key), zap.Error(err))
			writeError(w, http.StatusBadRequest, "plugin_unavailable", "Plugin is not available for this tenant")
			return
		}
		data, err := json.Marshal(body.Plugin.Data)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_plugin_data", "Invalid plugin data")
			return
		}
		o, err := h.orders.GetByID(ctx, id)
		if err != nil {
			writeEntError(w, err)
			return
		}
		if err := hook.ValidatePayment(ctx, o, data); err != nil {
			writeError(w, http.StatusBadRequest, "plugin_validation_failed", err.Error())
			return
		}
		pluginHook = hook
		pluginData = data
		pluginOrder = o
		pluginKey = &body.Plugin.Key
	} else if body.Method == generated.GratisPlugin {
		writeError(w, http.StatusBadRequest, "plugin_required", "gratis_plugin requires a plugin field")
		return
	}

	// A hook failure writes a >=400 status, so the request tx rolls back the payment.
	runRecordedHook := func() bool {
		if pluginHook == nil {
			return true
		}
		if err := pluginHook.OnPaymentRecorded(ctx, pluginOrder, string(body.Method), pluginData); err != nil {
			writeError(w, http.StatusBadRequest, "plugin_hook_failed", err.Error())
			return false
		}
		return true
	}

	paymentResponse := func(method string) map[string]any {
		resp := map[string]any{"orderId": id, "method": method}
		if pluginKey != nil {
			resp["pluginKey"] = *pluginKey
		}
		return resp
	}

	switch body.Method {
	case generated.Cash:
		var deviceID *string
		if did, ok := auth.GetDeviceID(ctx); ok {
			deviceID = &did
		}
		if err := h.pos.PayCash(ctx, id, deviceID, pluginKey); err != nil {
			writeError(w, http.StatusBadRequest, "payment_failed", err.Error())
			return
		}
		if !runRecordedHook() {
			return
		}
		resp := paymentResponse("cash")
		cachePaymentResponse(resp)
		response.WriteJSON(w, http.StatusCreated, resp)

	case generated.Card:
		var deviceID *string
		if did, ok := auth.GetDeviceID(ctx); ok {
			deviceID = &did
		}
		var card *repository.CardMeta
		if body.Card != nil {
			card = &repository.CardMeta{
				Brand:         body.Card.Brand,
				Last4:         body.Card.Last4,
				EntryMode:     body.Card.EntryMode,
				TransactionID: body.Card.TransactionId,
			}
		}
		if err := h.pos.PayCard(ctx, id, deviceID, card, pluginKey); err != nil {
			writeError(w, http.StatusBadRequest, "payment_failed", err.Error())
			return
		}
		if !runRecordedHook() {
			return
		}
		resp := paymentResponse("card")
		cachePaymentResponse(resp)
		response.WriteJSON(w, http.StatusCreated, resp)

	case generated.Twint:
		if twintPOS {
			var deviceID *string
			if did, ok := auth.GetDeviceID(ctx); ok {
				deviceID = &did
			}
			if err := h.pos.PayTwint(ctx, id, deviceID, pluginKey); err != nil {
				writeError(w, http.StatusBadRequest, "payment_failed", err.Error())
				return
			}
			if !runRecordedHook() {
				return
			}
			resp := paymentResponse("twint")
			resp["channel"] = "pos"
			cachePaymentResponse(resp)
			response.WriteJSON(w, http.StatusCreated, resp)
			return
		}

		o, err := h.orders.GetByID(ctx, id)
		if err != nil {
			writeEntError(w, err)
			return
		}

		returnURL := ""
		if body.ReturnUrl != nil {
			returnURL = *body.ReturnUrl
		}

		if !h.payments.IsPaymentConfigured(ctx) {
			h.logger.Warn("Payrexx not configured — simulating payment for dev",
				zap.String("orderId", id))
			if err := h.payments.MarkOrderPaidDev(ctx, id); err != nil {
				writeError(w, http.StatusInternalServerError, "dev_pay_failed", err.Error())
				return
			}
			resp := map[string]any{"orderId": id, "method": "twint", "redirectUrl": returnURL}
			cachePaymentResponse(resp)
			response.WriteJSON(w, http.StatusCreated, resp)
			return
		}

		prep := &service.CheckoutPreparation{
			OrderID:    o.ID,
			TotalCents: o.TotalCents,
		}
		failedURL := returnURL
		cancelURL := returnURL
		if returnURL != "" {
			sep := "?"
			if strings.Contains(returnURL, "?") {
				sep = "&"
			}
			failedURL = returnURL + sep + "result=failed"
			cancelURL = returnURL + sep + "result=cancel"
		}
		gw, err := h.payments.CreatePaymentCheckout(ctx, prep, returnURL, failedURL, cancelURL)
		if err != nil {
			h.logger.Error("payment checkout creation failed",
				zap.Error(err),
				zap.String("orderId", id),
				zap.Int64("totalCents", prep.TotalCents),
			)
			writeError(w, http.StatusBadGateway, "gateway_error", err.Error())
			return
		}
		resp := map[string]any{"orderId": id, "method": "twint", "redirectUrl": gw.Link, "gatewayId": gw.GatewayID}
		cachePaymentResponse(resp)
		response.WriteJSON(w, http.StatusCreated, resp)

	case generated.GratisGuest:
		var deviceID *string
		if did, ok := auth.GetDeviceID(ctx); ok {
			deviceID = &did
		}
		if err := h.pos.PayGratisGuest(ctx, id, deviceID); err != nil {
			writeError(w, http.StatusBadRequest, "payment_failed", err.Error())
			return
		}
		resp := map[string]any{"orderId": id, "method": "gratis_guest"}
		cachePaymentResponse(resp)
		response.WriteJSON(w, http.StatusCreated, resp)

	case generated.GratisVip:
		var deviceID *string
		if did, ok := auth.GetDeviceID(ctx); ok {
			deviceID = &did
		}
		if err := h.pos.PayGratisVIP(ctx, id, deviceID); err != nil {
			writeError(w, http.StatusBadRequest, "payment_failed", err.Error())
			return
		}
		resp := map[string]any{"orderId": id, "method": "gratis_vip"}
		cachePaymentResponse(resp)
		response.WriteJSON(w, http.StatusCreated, resp)

	case generated.GratisStaff:
		var deviceID *string
		if did, ok := auth.GetDeviceID(ctx); ok {
			deviceID = &did
		}
		if err := h.pos.PayGratisStaff(ctx, id, deviceID); err != nil {
			writeError(w, http.StatusBadRequest, "payment_failed", err.Error())
			return
		}
		resp := map[string]any{"orderId": id, "method": "gratis_staff"}
		cachePaymentResponse(resp)
		response.WriteJSON(w, http.StatusCreated, resp)

	case generated.GratisPlugin:
		var deviceID *string
		if did, ok := auth.GetDeviceID(ctx); ok {
			deviceID = &did
		}
		if err := h.pos.PayGratisPlugin(ctx, id, deviceID, *pluginKey); err != nil {
			writeError(w, http.StatusBadRequest, "payment_failed", err.Error())
			return
		}
		if !runRecordedHook() {
			return
		}
		resp := paymentResponse("gratis_plugin")
		cachePaymentResponse(resp)
		response.WriteJSON(w, http.StatusCreated, resp)

	default:
		writeError(w, http.StatusBadRequest, "invalid_method", "Unsupported payment method")
	}
}

// ListEvents returns days with paid orders for admin dashboard navigation.
// (GET /events)
func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.orders.ListEvents(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}

	items := make([]generated.Event, 0, len(events))
	for _, e := range events {
		items = append(items, generated.Event{
			Year:       e.Year,
			Month:      e.Month,
			Day:        e.Day,
			OrderCount: e.OrderCount,
		})
	}
	response.WriteJSON(w, http.StatusOK, generated.EventList{Items: items})
}
