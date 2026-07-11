package artistvoucher

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"

	"backend/internal/pdf"

	"go.uber.org/zap"
)

// ArtistVoucherPrint streams a PDF of the tenant's active and scheduled vouchers,
// one byte-mode QR per cell. GET /plugins/artist-voucher/vouchers/print.pdf?cols=
func (h *handlers) ArtistVoucherPrint(w http.ResponseWriter, r *http.Request) {
	cols := pdf.VoucherDefaultCols
	if v := r.URL.Query().Get("cols"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < pdf.VoucherMinCols || n > pdf.VoucherMaxCols {
			writeError(w, http.StatusBadRequest, "invalid_cols",
				fmt.Sprintf("cols must be between %d and %d", pdf.VoucherMinCols, pdf.VoucherMaxCols))
			return
		}
		cols = n
	}

	vouchers, err := h.rt.svc.List(r.Context())
	if err != nil {
		h.rt.logger.Error("artist voucher print list failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not load vouchers")
		return
	}

	slips := make([]pdf.VoucherSlip, 0, len(vouchers))
	for _, v := range vouchers {
		if v.Status != StatusActive && v.Status != StatusScheduled {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(v.Token)
		if err != nil {
			h.rt.logger.Error("artist voucher token decode failed", zap.String("voucher_id", v.ID), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not render voucher")
			return
		}
		names := make([]string, 0, len(v.Categories))
		for _, c := range v.Categories {
			names = append(names, c.Name)
		}
		slips = append(slips, pdf.VoucherSlip{Label: v.Label, Categories: names, QRBytes: raw})
	}

	body, err := pdf.RenderVoucherSlips(slips, cols)
	if err != nil {
		h.rt.logger.Error("render artist voucher slips", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "render_failed", "Could not render vouchers")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="artist-vouchers.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}
