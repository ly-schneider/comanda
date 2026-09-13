package artistvoucher

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

	brandName, brandIcon := h.brandHeader(r.Context())

	body, err := pdf.RenderVoucherSlips(pdf.VoucherSlip{
		Label:        voucher.Label,
		Details:      names,
		QRBytes:      raw,
		BrandName:    brandName,
		BrandIconPNG: brandIcon,
	}, count)
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

// brandHeader resolves the tenant's display name and a PNG-encoded icon (favicon,
// else logo) for the badge header. Branding is best-effort: any failure degrades
// to a name-only or bare badge rather than failing the print.
func (h *handlers) brandHeader(ctx context.Context) (name string, iconPNG []byte) {
	branding, err := h.rt.branding.GetBranding(ctx)
	if err != nil {
		h.rt.logger.Warn("artist voucher print: load branding failed", zap.Error(err))
		return "", nil
	}
	name = branding.DisplayName

	iconURL := ""
	if branding.FaviconURL != nil && strings.TrimSpace(*branding.FaviconURL) != "" {
		iconURL = strings.TrimSpace(*branding.FaviconURL)
	} else if branding.LogoURL != nil {
		iconURL = strings.TrimSpace(*branding.LogoURL)
	}
	if iconURL == "" || h.rt.blob == nil {
		return name, nil
	}

	raw, err := h.rt.blob.Download(ctx, iconURL)
	if err != nil {
		h.rt.logger.Warn("artist voucher print: download brand icon failed", zap.String("url", iconURL), zap.Error(err))
		return name, nil
	}
	png, err := decodeToPNG(raw)
	if err != nil {
		h.rt.logger.Warn("artist voucher print: decode brand icon failed", zap.Error(err))
		return name, nil
	}
	return name, png
}

func (h *handlers) ArtistVoucherBatchPrint(w http.ResponseWriter, r *http.Request, batchId string) {
	if !nanoid.Valid(batchId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid batch id")
		return
	}

	detail, err := h.rt.svc.GetBatch(r.Context(), batchId)
	if err != nil {
		h.writeBatchError(w, "print", err)
		return
	}

	details := make([]string, 0, len(detail.Products))
	for _, p := range detail.Products {
		details = append(details, fmt.Sprintf("%d× %s", p.Quantity, p.Name))
	}
	brandName, brandIcon := h.brandHeader(r.Context())

	slips := make([]pdf.VoucherSlip, 0, len(detail.Vouchers))
	for _, v := range detail.Vouchers {
		// A spent or revoked slip would be refused at the stand, so reprinting a
		// batch must not put one back in someone's hand.
		if v.Status != StatusActive && v.Status != StatusScheduled {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(v.Token)
		if err != nil {
			h.rt.logger.Error("artist voucher token decode failed", zap.String("voucher_id", v.ID), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "artist_voucher_error", "Could not render vouchers")
			return
		}
		slips = append(slips, pdf.VoucherSlip{
			Label:        detail.Label,
			Details:      details,
			QRBytes:      raw,
			BrandName:    brandName,
			BrandIconPNG: brandIcon,
		})
	}

	if len(slips) == 0 {
		writeError(w, http.StatusConflict, "no_printable_vouchers", "Alle Gutscheine dieser Charge sind eingelöst oder widerrufen.")
		return
	}

	body, err := pdf.RenderVoucherBatchSlips(slips)
	if err != nil {
		h.rt.logger.Error("render artist voucher batch slips", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "render_failed", "Could not render vouchers")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="gutscheine-%s.pdf"`, detail.ID))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}
