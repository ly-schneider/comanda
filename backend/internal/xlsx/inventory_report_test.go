package xlsx

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func testReport() InventoryReport {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	return InventoryReport{
		ScopeLabel: "Bar Ost",
		From:       from,
		To:         from.AddDate(0, 0, 2),
		Summary: []SummaryRow{
			{Category: "Getränke", Subcategory: "Bar Ost", Product: "Bier", Variant: "Gross",
				Opening: 100, Sold: 42, Refunded: 2, Cancelled: 1, Manual: -3, Closing: 58},
		},
		Movements: []MovementRow{
			{At: from.Add(3 * time.Hour), Category: "Getränke", Subcategory: "Bar Ost",
				Product: "Bier", Variant: "Gross", Delta: -2, Reason: "Verkauf", OrderID: "ord000000001"},
		},
	}
}

func openRendered(t *testing.T, body []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("rendered workbook does not open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestRenderInventoryReportSheets(t *testing.T) {
	body, err := RenderInventoryReport(testReport())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	sheets := f.GetSheetList()
	if len(sheets) != 2 || sheets[0] != sheetSummary || sheets[1] != sheetMovements {
		t.Fatalf("expected [%s %s], got %v", sheetSummary, sheetMovements, sheets)
	}
}

func TestRenderInventoryReportSummaryCells(t *testing.T) {
	body, err := RenderInventoryReport(testReport())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	// Row 4 is the header, row 5 the first data row.
	for col, want := range map[string]string{
		"A4": "Kategorie", "E4": "Anfangsbestand", "F4": "Verkauft", "J4": "Endbestand",
		"C5": "Bier", "D5": "Gross", "E5": "100", "F5": "42", "J5": "58",
	} {
		got, err := f.GetCellValue(sheetSummary, col)
		if err != nil {
			t.Fatalf("read %s: %v", col, err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", col, got, want)
		}
	}

	title, _ := f.GetCellValue(sheetSummary, "A1")
	if title != "Inventarverlauf Bar Ost · 01.07.2026 bis 02.07.2026" {
		t.Fatalf("title = %q", title)
	}
}

func TestRenderInventoryReportMovementCells(t *testing.T) {
	body, err := RenderInventoryReport(testReport())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	for col, want := range map[string]string{
		"A1": "Datum", "F1": "Delta", "I1": "Erfasst von",
		"D2": "Bier", "F2": "-2", "G2": "Verkauf", "H2": "ord000000001",
	} {
		got, err := f.GetCellValue(sheetMovements, col)
		if err != nil {
			t.Fatalf("read %s: %v", col, err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", col, got, want)
		}
	}
}

func TestRenderInventoryReportEmptyScope(t *testing.T) {
	report := testReport()
	report.Summary = nil
	report.Movements = nil

	body, err := RenderInventoryReport(report)
	if err != nil {
		t.Fatalf("empty report must still render: %v", err)
	}
	f := openRendered(t, body)

	if got, _ := f.GetCellValue(sheetSummary, "A4"); got != "Kategorie" {
		t.Fatalf("headers must survive an empty report, got %q", got)
	}
}
