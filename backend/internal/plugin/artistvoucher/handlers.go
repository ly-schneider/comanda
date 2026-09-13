package artistvoucher

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	nanoid "backend/internal/id"
	"backend/internal/repository"
	"backend/internal/response"
	coreservice "backend/internal/service"

	"go.uber.org/zap"
)

type handlers struct {
	generated.Unimplemented
	rt *runtime
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{Code: code, Message: message})
}

func toAPIVoucher(v Voucher) generated.ArtistVoucher {
	cats := make([]generated.ArtistVoucherCategory, 0, len(v.Categories))
	for _, c := range v.Categories {
		cats = append(cats, generated.ArtistVoucherCategory{Id: c.ID, Name: c.Name})
	}
	return generated.ArtistVoucher{
		Id:         v.ID,
		Label:      v.Label,
		ValidFrom:  v.ValidFrom,
		ValidUntil: v.ValidUntil,
		RevokedAt:  v.RevokedAt,
		Categories: cats,
		Token:      v.Token,
		Status:     generated.ArtistVoucherStatus(v.Status),
		CreatedAt:  v.CreatedAt,
	}
}

func (h *handlers) ArtistVoucherList(w http.ResponseWriter, r *http.Request) {
	vouchers, err := h.rt.svc.List(r.Context())
	if err != nil {
		h.rt.logger.Error("artist voucher list failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not load vouchers")
		return
	}
	out := make([]generated.ArtistVoucher, 0, len(vouchers))
	for _, v := range vouchers {
		out = append(out, toAPIVoucher(v))
	}
	response.WriteJSON(w, http.StatusOK, generated.ArtistVoucherListResponse{Vouchers: out})
}

func (h *handlers) ArtistVoucherCreate(w http.ResponseWriter, r *http.Request) {
	var body generated.ArtistVoucherCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	for _, cid := range body.CategoryIds {
		if !nanoid.Valid(cid) {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid category id")
			return
		}
	}

	v, err := h.rt.svc.Create(r.Context(), CreateInput{
		Label:       body.Label,
		ValidFrom:   body.ValidFrom,
		ValidUntil:  body.ValidUntil,
		CategoryIDs: body.CategoryIds,
	})
	if err != nil {
		if IsValidation(err) {
			writeError(w, http.StatusBadRequest, "invalid_voucher", err.Error())
			return
		}
		h.rt.logger.Error("artist voucher create failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not create voucher")
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIVoucher(v))
}

func (h *handlers) ArtistVoucherRevoke(w http.ResponseWriter, r *http.Request, voucherId string) {
	if !nanoid.Valid(voucherId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid voucher id")
		return
	}
	v, err := h.rt.svc.Revoke(r.Context(), voucherId)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Voucher not found")
			return
		}
		h.rt.logger.Error("artist voucher revoke failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not revoke voucher")
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIVoucher(v))
}

func (h *handlers) ArtistVoucherDelete(w http.ResponseWriter, r *http.Request, voucherId string) {
	if !nanoid.Valid(voucherId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid voucher id")
		return
	}
	err := h.rt.svc.Delete(r.Context(), voucherId)
	switch {
	case err == nil:
		response.WriteNoContent(w)
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Voucher not found")
	case errors.Is(err, ErrNotExpired):
		writeError(w, http.StatusConflict, "voucher_not_expired", "Only expired vouchers can be deleted")
	default:
		h.rt.logger.Error("artist voucher delete failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not delete voucher")
	}
}

func (h *handlers) ArtistVoucherRevocations(w http.ResponseWriter, r *http.Request) {
	feed, err := h.rt.svc.Revocations(r.Context())
	if err != nil {
		h.rt.logger.Error("artist voucher revocations failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not load revocations")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, generated.ArtistVoucherRevocations{
		RevokedIds: feed.RevokedIDs,
		AsOf:       feed.AsOf,
	})
}

func toAPIBatch(b Batch) generated.ArtistVoucherBatch {
	products := make([]generated.ArtistVoucherBatchProduct, 0, len(b.Products))
	for _, p := range b.Products {
		products = append(products, generated.ArtistVoucherBatchProduct{
			ProductId:   p.ID,
			Name:        p.Name,
			Quantity:    p.Quantity,
			ChoiceGroup: &p.ChoiceGroup,
			Stock:       p.Stock,
		})
	}
	return generated.ArtistVoucherBatch{
		Id:          b.ID,
		Label:       b.Label,
		ValidFrom:   b.ValidFrom,
		ValidUntil:  b.ValidUntil,
		Products:    products,
		Issued:      b.Issued,
		Redeemed:    b.Redeemed,
		Revoked:     b.Revoked,
		Outstanding: b.Outstanding,
		Status:      generated.ArtistVoucherBatchStatus(b.Status),
		CreatedAt:   b.CreatedAt,
	}
}

func batchErrorResponse(err error) (status int, code, message string, ok bool) {
	switch {
	case errors.Is(err, ErrEmptyLabel):
		return http.StatusBadRequest, "label_required", "Bitte gib eine Bezeichnung an.", true
	case errors.Is(err, ErrLabelTooLong):
		return http.StatusBadRequest, "label_too_long", "Die Bezeichnung darf höchstens 80 Zeichen lang sein.", true
	case errors.Is(err, ErrInvalidWindow):
		return http.StatusBadRequest, "invalid_window", "Das Enddatum muss nach dem Startdatum liegen.", true
	case errors.Is(err, ErrInvalidCount):
		return http.StatusBadRequest, "invalid_count", fmt.Sprintf("Die Anzahl muss zwischen %d und %d liegen.", minBatchCount, maxBatchCount), true
	case errors.Is(err, ErrNoProducts):
		return http.StatusBadRequest, "no_products", "Wähle mindestens ein Produkt aus.", true
	case errors.Is(err, ErrTooManyProducts):
		return http.StatusBadRequest, "too_many_products", fmt.Sprintf("Ein Gutschein kann höchstens %d Produkte enthalten.", maxBatchProducts), true
	case errors.Is(err, ErrDuplicateProduct):
		return http.StatusBadRequest, "duplicate_product", "Jedes Produkt darf nur einmal im Gutschein vorkommen.", true
	case errors.Is(err, ErrUnknownProduct):
		return http.StatusBadRequest, "unknown_product", "Das gewählte Produkt existiert nicht.", true
	case errors.Is(err, ErrInvalidQuantity):
		return http.StatusBadRequest, "invalid_quantity", fmt.Sprintf("Die Menge muss zwischen 1 und %d liegen.", maxBatchQuantity), true
	case errors.Is(err, ErrProductNotSimple):
		return http.StatusBadRequest, "product_not_simple", "Für Gutscheine lassen sich nur einfache Produkte auswählen, keine Menüs.", true
	case errors.Is(err, ErrProductHasVariants):
		return http.StatusBadRequest, "product_has_variants", "Produkte mit Varianten können nicht als Gutschein ausgegeben werden, weil auf dem Gutschein keine Variante steht.", true
	case errors.Is(err, ErrInvalidChoiceGroup):
		return http.StatusBadRequest, "invalid_choice_group", "Die Auswahlgruppe liegt ausserhalb des gültigen Bereichs.", true
	case errors.Is(err, ErrChoiceGroupTooSmall):
		return http.StatusBadRequest, "choice_group_too_small", "Eine Auswahl braucht mindestens zwei Produkte, sonst gibt es nichts zu wählen.", true
	case errors.Is(err, ErrProductUnavailable):
		return http.StatusBadRequest, "product_unavailable", "Das Produkt ist nicht im Verkauf. Aktiviere es zuerst, sonst lassen sich die Gutscheine am Stand nicht einlösen.", true
	case errors.Is(err, ErrInvalidToken):
		return http.StatusBadRequest, "invalid_voucher", "Der Gutschein passt nicht in das QR-Format.", true
	case errors.Is(err, repository.ErrNotFound):
		return http.StatusNotFound, "not_found", "Die Gutschein-Serie existiert nicht.", true
	default:
		return 0, "", "", false
	}
}

func (h *handlers) writeBatchError(w http.ResponseWriter, action string, err error) {
	if status, code, message, ok := batchErrorResponse(err); ok {
		writeError(w, status, code, message)
		return
	}
	h.rt.logger.Error("artist voucher batch "+action+" failed", zap.Error(err))
	writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Die Gutschein-Serie konnte nicht verarbeitet werden")
}

func (h *handlers) ArtistVoucherBatchList(w http.ResponseWriter, r *http.Request) {
	batches, err := h.rt.svc.ListBatches(r.Context())
	if err != nil {
		h.writeBatchError(w, "list", err)
		return
	}
	out := make([]generated.ArtistVoucherBatch, 0, len(batches))
	for _, b := range batches {
		out = append(out, toAPIBatch(b))
	}
	response.WriteJSON(w, http.StatusOK, generated.ArtistVoucherBatchListResponse{Batches: out})
}

func (h *handlers) ArtistVoucherBatchCreate(w http.ResponseWriter, r *http.Request) {
	var body generated.ArtistVoucherBatchCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	products := make([]BatchProductInput, 0, len(body.Products))
	for _, p := range body.Products {
		if !nanoid.Valid(p.ProductId) {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid product id")
			return
		}
		group := 0
		if p.ChoiceGroup != nil {
			group = *p.ChoiceGroup
		}
		products = append(products, BatchProductInput{ProductID: p.ProductId, Quantity: p.Quantity, ChoiceGroup: group})
	}

	batch, err := h.rt.svc.CreateBatch(r.Context(), CreateBatchInput{
		Label:      body.Label,
		ValidFrom:  body.ValidFrom,
		ValidUntil: body.ValidUntil,
		Products:   products,
		Count:      body.Count,
	})
	if err != nil {
		h.writeBatchError(w, "create", err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIBatch(batch))
}

func (h *handlers) ArtistVoucherBatchGet(w http.ResponseWriter, r *http.Request, batchId string) {
	if !nanoid.Valid(batchId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid batch id")
		return
	}
	detail, err := h.rt.svc.GetBatch(r.Context(), batchId)
	if err != nil {
		h.writeBatchError(w, "get", err)
		return
	}
	vouchers := make([]generated.ArtistVoucherBatchVoucher, 0, len(detail.Vouchers))
	for _, v := range detail.Vouchers {
		vouchers = append(vouchers, generated.ArtistVoucherBatchVoucher{
			Id:         v.ID,
			Status:     generated.ArtistVoucherBatchVoucherStatus(v.Status),
			RedeemedAt: v.RedeemedAt,
			RevokedAt:  v.RevokedAt,
		})
	}
	response.WriteJSON(w, http.StatusOK, generated.ArtistVoucherBatchDetail{
		Batch:    toAPIBatch(detail.Batch),
		Vouchers: vouchers,
	})
}

func (h *handlers) ArtistVoucherBatchRevoke(w http.ResponseWriter, r *http.Request, batchId string) {
	if !nanoid.Valid(batchId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid batch id")
		return
	}
	batch, err := h.rt.svc.RevokeBatch(r.Context(), batchId)
	if err != nil {
		h.writeBatchError(w, "revoke", err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIBatch(batch))
}

func (h *handlers) ArtistVoucherRedeem(w http.ResponseWriter, r *http.Request, voucherId string) {
	if !nanoid.Valid(voucherId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid voucher id")
		return
	}
	deviceID, _ := auth.GetDeviceID(r.Context())

	var body generated.ArtistVoucherRedeemRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	var selections []ChoiceSelection
	if body.Selections != nil {
		for _, sel := range *body.Selections {
			selections = append(selections, ChoiceSelection{ChoiceGroup: sel.ChoiceGroup, ProductID: sel.ProductId})
		}
	}

	result, err := h.rt.svc.Redeem(r.Context(), voucherId, deviceID, selections)
	if err != nil {
		h.writeRedeemError(w, err)
		return
	}

	items := make([]generated.ArtistVoucherRedeemItem, 0, len(result.Items))
	for _, it := range result.Items {
		items = append(items, generated.ArtistVoucherRedeemItem{
			ProductId: it.ID,
			Name:      it.Name,
			Quantity:  it.Quantity,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, generated.ArtistVoucherRedeemResponse{
		VoucherId:       result.VoucherID,
		Label:           result.Label,
		OrderId:         result.OrderID,
		RedeemedAt:      result.RedeemedAt,
		AlreadyRedeemed: result.AlreadyRedeemed,
		Items:           items,
	})
}

func (h *handlers) writeRedeemError(w http.ResponseWriter, err error) {
	var needsChoice *ChoiceRequiredError
	if errors.As(err, &needsChoice) {
		groups := make([]map[string]any, 0, len(needsChoice.Groups))
		for _, g := range needsChoice.Groups {
			options := make([]map[string]any, 0, len(g.Options))
			for _, o := range g.Options {
				options = append(options, map[string]any{"productId": o.ID, "name": o.Name, "quantity": o.Quantity})
			}
			groups = append(groups, map[string]any{"choiceGroup": g.ChoiceGroup, "options": options})
		}
		details := map[string]any{"groups": groups}
		response.WriteJSON(w, http.StatusConflict, generated.Error{
			Code:    "voucher_choice_required",
			Message: "Für diesen Gutschein muss noch eine Auswahl getroffen werden.",
			Details: &details,
		})
		return
	}

	var soldOut *coreservice.InsufficientInventoryError
	if errors.As(err, &soldOut) {
		items := make([]map[string]any, 0, len(soldOut.Items))
		names := make([]string, 0, len(soldOut.Items))
		for _, it := range soldOut.Items {
			items = append(items, map[string]any{
				"productId": it.ProductID,
				"name":      it.Name,
				"available": it.Available,
				"requested": it.Requested,
			})
			names = append(names, it.Name)
		}
		details := map[string]any{"items": items}
		response.WriteJSON(w, http.StatusConflict, generated.Error{
			Code:    "voucher_product_sold_out",
			Message: "Der Gutschein ist gültig, aber ausverkauft ist: " + strings.Join(names, ", "),
			Details: &details,
		})
		return
	}

	var unavailable *coreservice.ProductUnavailableError
	if errors.As(err, &unavailable) {
		names := make([]string, 0, len(unavailable.Items))
		for _, it := range unavailable.Items {
			names = append(names, it.Name)
		}
		writeError(w, http.StatusConflict, "voucher_product_unavailable",
			"Der Gutschein ist gültig, aber nicht mehr im Sortiment ist: "+strings.Join(names, ", "))
		return
	}

	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Dieser Gutschein ist unbekannt.")
	case errors.Is(err, ErrInvalidSelection):
		writeError(w, http.StatusBadRequest, "voucher_invalid_selection",
			"Die getroffene Auswahl passt nicht zu diesem Gutschein.")
	case errors.Is(err, ErrNotEntitlement):
		writeError(w, http.StatusBadRequest, "voucher_not_single_use", "Dieser Gutschein kann nicht eingelöst werden.")
	case errors.Is(err, ErrRevoked):
		writeError(w, http.StatusConflict, "voucher_revoked", "Dieser Gutschein wurde gesperrt.")
	case errors.Is(err, ErrNotYetValid):
		writeError(w, http.StatusConflict, "voucher_not_yet_valid", "Dieser Gutschein ist noch nicht gültig.")
	case errors.Is(err, ErrExpired):
		writeError(w, http.StatusConflict, "voucher_expired", "Dieser Gutschein ist abgelaufen.")
	default:
		h.rt.logger.Error("artist voucher redeem failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Der Gutschein konnte nicht eingelöst werden")
	}
}
