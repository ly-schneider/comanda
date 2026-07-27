package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	generated "backend/internal/generated/api/generated"
	"backend/internal/service"
	"backend/internal/xlsx"
)

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ExportInventory streams a two-sheet Excel workbook of the inventory history.
// (GET /inventory/export.xlsx)
func (h *Handlers) ExportInventory(w http.ResponseWriter, r *http.Request, params generated.ExportInventoryParams) {
	ctx := r.Context()

	report, err := h.inventoryExport.Build(
		ctx,
		service.InventoryExportScope(params.Scope),
		params.ScopeId,
		params.From,
		params.To,
	)
	if err != nil {
		if errors.Is(err, service.ErrInvalidExportScope) {
			writeError(w, http.StatusBadRequest, "invalid_scope", err.Error())
			return
		}
		writeEntError(w, err)
		return
	}

	body, err := xlsx.RenderInventoryReport(report)
	if err != nil {
		h.logger.Error("inventory export render failed")
		writeError(w, http.StatusInternalServerError, "export_failed", "Export konnte nicht erstellt werden")
		return
	}

	w.Header().Set("Content-Type", xlsxContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", exportFilename(report)))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

func exportFilename(report xlsx.InventoryReport) string {
	name := slugForFilename(report.ScopeLabel)
	if name == "" {
		name = "Export"
	}
	return fmt.Sprintf("Inventar_%s_%s.xlsx", name, report.From.Format("2006-01-02"))
}

var filenameTransliterations = strings.NewReplacer(
	"ä", "ae", "ö", "oe", "ü", "ue",
	"Ä", "Ae", "Ö", "Oe", "Ü", "Ue",
	"ß", "ss", "à", "a", "é", "e", "è", "e", "ê", "e", "ô", "o", "û", "u", "ç", "c",
)

// Content-Disposition filenames stay ASCII so every client agrees on the name.
func slugForFilename(s string) string {
	var b strings.Builder
	for _, r := range filenameTransliterations.Replace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
