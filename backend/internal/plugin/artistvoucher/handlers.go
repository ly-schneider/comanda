package artistvoucher

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/generated/api/generated"
	nanoid "backend/internal/id"
	"backend/internal/repository"
	"backend/internal/response"

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
