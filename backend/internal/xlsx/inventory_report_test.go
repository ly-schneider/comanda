package xlsx

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// The client picks local days and ships them as UTC instants, so fixtures must
// too — UTC-midnight input would hide any timezone slip in the rendered dates.
func swissDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, reportLocation).UTC()
}

func testReport() InventoryReport {
	from := swissDay(2026, 7, 1)
	return InventoryReport{
		ScopeLabel: "Bar Ost",
		From:       from,
		To:         swissDay(2026, 7, 3),
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

func TestRenderInventoryReportUsesSwissWallClock(t *testing.T) {
	// A single Swiss day: 00:00 local is 22:00 UTC the previous day.
	from := swissDay(2026, 7, 27)
	report := testReport()
	report.From = from
	report.To = swissDay(2026, 7, 28)
	report.Movements = []MovementRow{{At: from, Product: "Bier", Delta: -1, Reason: "Verkauf"}}

	body, err := RenderInventoryReport(report)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	title, _ := f.GetCellValue(sheetSummary, "A1")
	if title != "Inventarverlauf Bar Ost · 27.07.2026 bis 27.07.2026" {
		t.Fatalf("a one-day range must read as that day in both bounds, got %q", title)
	}

	at, _ := f.GetCellValue(sheetMovements, "A2")
	if at != "27.07.2026 00:00" {
		t.Fatalf("movement timestamp must render in Swiss wall-clock time, got %q", at)
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
