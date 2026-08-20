package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	generated "backend/internal/generated/api/generated"
	"backend/internal/response"
	"backend/internal/service"
	"backend/internal/xlsx"
)

// ExportSales streams the sales and revenue workbook for a date range.
// (GET /sales/export.xlsx)
func (h *Handlers) ExportSales(w http.ResponseWriter, r *http.Request, params generated.ExportSalesParams) {
	report, err := h.salesExport.Build(r.Context(), params.From, params.To)
	if err != nil {
		if errors.Is(err, service.ErrInvalidExportRange) {
			writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
			return
		}
		writeEntError(w, err)
		return
	}

	body, err := xlsx.RenderSalesReport(report)
	if err != nil {
		h.logger.Error("sales export render failed")
		writeError(w, http.StatusInternalServerError, "export_failed", "Export konnte nicht erstellt werden")
		return
	}

	w.Header().Set("Content-Type", xlsxContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", salesExportFilename(report)))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

// GetSalesSummary previews what the export contains, over the same range.
// (GET /sales/summary)
func (h *Handlers) GetSalesSummary(w http.ResponseWriter, r *http.Request, params generated.GetSalesSummaryParams) {
	report, err := h.salesExport.Build(r.Context(), params.From, params.To)
	if err != nil {
		if errors.Is(err, service.ErrInvalidExportRange) {
			writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
			return
		}
		writeEntError(w, err)
		return
	}

	articles := make([]generated.SalesArticle, 0, len(report.Articles))
	for _, a := range report.Articles {
		articles = append(articles, generated.SalesArticle{
			Category:      optionalString(a.Category),
			Subcategory:   optionalString(a.Subcategory),
			Product:       a.Product,
			Variant:       optionalString(a.Variant),
			Quantity:      a.Quantity,
			BundleQty:     a.BundleQty,
			GratisQty:     a.GratisQty,
			RefundedQty:   a.RefundedQty,
			RevenueCents:  a.RevenueCents,
			DepositCents:  a.DepositCents,
			RefundedCents: a.RefundedCents,
			NetCents:      a.NetCents(),
			GratisCents:   a.GratisCents,
		})
	}

	methods := make([]generated.SalesPaymentMethod, 0, len(report.Methods))
	for _, m := range report.Methods {
		methods = append(methods, generated.SalesPaymentMethod{
			Method:        m.Method,
			OrderCount:    m.OrderCount,
			RevenueCents:  m.RevenueCents,
			DepositCents:  m.DepositCents,
			RefundedCents: m.RefundedCents,
			NetCents:      m.NetCents(),
			GratisCents:   m.GratisCents,
		})
	}

	response.WriteJSON(w, http.StatusOK, generated.SalesSummary{
		Totals: generated.SalesTotals{
			OrderCount:       report.Totals.OrderCount,
			ItemCount:        report.Totals.ItemCount,
			RevenueCents:     report.Totals.RevenueCents,
			DepositCents:     report.Totals.DepositCents,
			RefundedCents:    report.Totals.RefundedCents,
			NetCents:         report.NetCents(),
			GratisCents:      report.Totals.GratisCents,
			GratisOrderCount: report.Totals.GratisOrderCount,
			GratisItemCount:  report.Totals.GratisItemCount,
		},
		Methods:  methods,
		Articles: articles,
	})
}

func salesExportFilename(report xlsx.SalesReport) string {
	return fmt.Sprintf("Verkaeufe_%s.xlsx", report.From.In(xlsx.Location()).Format("2006-01-02"))
}
