package artistvoucher

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"backend/internal/generated/api/generated"
	"backend/internal/pdf"
	"backend/internal/repository"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

// ArtistVoucherPrint streams a PDF of count identical badge-sized slips for one voucher.
func (h *handlers) ArtistVoucherPrint(w http.ResponseWriter, r *http.Request, voucherId string, params generated.ArtistVoucherPrintParams) {
	if !nanoid.Valid(voucherId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid voucher id")
		return
	}

	count := pdf.VoucherDefaultCount
	if params.Count != nil {
		if n := *params.Count; n < pdf.VoucherMinCount || n > pdf.VoucherMaxCount {
			writeError(w, http.StatusBadRequest, "invalid_count",
				fmt.Sprintf("count must be between %d and %d", pdf.VoucherMinCount, pdf.VoucherMaxCount))
			return
		}
		count = *params.Count
	}

	voucher, err := h.rt.svc.Get(r.Context(), voucherId)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Voucher not found")
			return
		}
		h.rt.logger.Error("artist voucher print get failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not load voucher")
		return
	}

	raw, err := base64.RawURLEncoding.DecodeString(voucher.Token)
	if err != nil {
		h.rt.logger.Error("artist voucher token decode failed", zap.String("voucher_id", voucher.ID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not render voucher")
		return
	}
	names := make([]string, 0, len(voucher.Categories))
	for _, c := range voucher.Categories {
		names = append(names, c.Name)
	}

	body, err := pdf.RenderVoucherSlips(pdf.VoucherSlip{Label: voucher.Label, Categories: names, QRBytes: raw}, count)
	if err != nil {
		h.rt.logger.Error("render artist voucher slips", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "render_failed", "Could not render vouchers")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="artist-voucher-%s.pdf"`, voucher.ID))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}
