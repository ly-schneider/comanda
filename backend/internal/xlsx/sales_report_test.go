package xlsx

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func testSalesReport() SalesReport {
	from := swissDay(2026, 7, 24)
	return SalesReport{
		From: from,
		To:   swissDay(2026, 7, 25),
		Totals: SalesTotals{
			OrderCount: 2, ItemCount: 3,
			RevenueCents: 8400, DepositCents: 400, RefundedCents: 500,
			UnlinkedRefunds: 500, GratisCents: 3000,
			GratisOrderCount: 1, GratisItemCount: 1,
		},
		Methods: []MethodRow{
			{Method: "Bargeld", OrderCount: 1, RevenueCents: 6000, DepositCents: 400, RefundedCents: 500},
			{Method: "TWINT", OrderCount: 1, RevenueCents: 2400},
		},
		Articles: []ArticleRow{
			{Category: "Merch", Product: "Shirt", Quantity: 3, BundleQty: 1, GratisQty: 1,
				RefundedQty: 1, RevenueCents: 8400, DepositCents: 400, RefundedCents: 500, GratisCents: 3000},
		},
		Sales: []SaleRow{
			{At: from.Add(19 * time.Hour), OrderID: "ord000000001", Channel: "POS",
				PaymentMethod: "Bargeld", Category: "Merch", Product: "Shirt", Source: "Einzeln",
				Quantity: 2, UnitCents: 3000, RevenueCents: 6000, DepositCents: 400, RefundedCents: 500},
			{At: from.Add(20 * time.Hour), OrderID: "ord000000002", Channel: "Shop",
				PaymentMethod: "TWINT", Category: "Merch", Product: "Shirt",
				Source: "Bundle: Shirt + Getränk", FromBundle: true,
				Quantity: 1, UnitCents: 2400, RevenueCents: 2400},
		},
		Orders: []OrderRow{
			{At: from.Add(19 * time.Hour), OrderID: "ord000000001", Channel: "POS", Status: "Bezahlt",
				PaymentMethod: "Bargeld", ItemCount: 2, TotalCents: 6400, RefundedCents: 500},
		},
	}
}

func TestRenderSalesReportSheets(t *testing.T) {
	body, err := RenderSalesReport(testSalesReport())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	want := []string{salesSheetSummary, salesSheetArticles, salesSheetLines, salesSheetOrders}
	sheets := f.GetSheetList()
	if len(sheets) != len(want) {
		t.Fatalf("expected %v, got %v", want, sheets)
	}
	for i, name := range want {
		if sheets[i] != name {
			t.Fatalf("sheet %d = %q, want %q", i, sheets[i], name)
		}
	}
}

func TestRenderSalesReportSummaryCells(t *testing.T) {
	body, err := RenderSalesReport(testSalesReport())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	for ref, want := range map[string]string{
		"A1": "Verkäufe 24.07.2026",
		"A3": "Bestellungen",
		"B3": "2",
		"A5": "Warenumsatz",
		"B5": "84.00",
		"A7": "Erstattungen",
		"B7": "-5.00",
		"A8": "Nettoeinnahmen",
		"B8": "83.00",
		"A9": "Gratis abgegeben (Warenwert)",
		"B9": "30.00",
	} {
		got, err := f.GetCellValue(salesSheetSummary, ref)
		if err != nil {
			t.Fatalf("read %s: %v", ref, err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", ref, got, want)
		}
	}
}

func TestRenderSalesReportWritesAmountsAsNumbers(t *testing.T) {
	body, err := RenderSalesReport(testSalesReport())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	for sheet, want := range map[string]struct{ ref, raw, formatted string }{
		salesSheetLines:    {"M2", "60", "60.00"},
		salesSheetArticles: {"I5", "84", "84.00"},
		salesSheetOrders:   {"G2", "64", "64.00"},
	} {
		raw, err := f.GetCellValue(sheet, want.ref, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("raw %s!%s: %v", sheet, want.ref, err)
		}
		if raw != want.raw {
			t.Fatalf("%s!%s stores %q, want the bare number %q", sheet, want.ref, raw, want.raw)
		}
		formatted, err := f.GetCellValue(sheet, want.ref)
		if err != nil {
			t.Fatalf("read %s!%s: %v", sheet, want.ref, err)
		}
		if formatted != want.formatted {
			t.Fatalf("%s!%s renders as %q, want %q", sheet, want.ref, formatted, want.formatted)
		}
	}
}

func TestRenderSalesReportBundleOriginSurvives(t *testing.T) {
	body, err := RenderSalesReport(testSalesReport())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)

	got, err := f.GetCellValue(salesSheetLines, "I3")
	if err != nil {
		t.Fatalf("read origin: %v", err)
	}
	if got != "Bundle: Shirt + Getränk" {
		t.Fatalf("Herkunft = %q", got)
	}
	if got, _ := f.GetCellValue(salesSheetArticles, "F5"); got != "1" {
		t.Fatalf("davon über Bundle = %q, want 1", got)
	}
}

func TestRenderSalesReportHandlesEmptyRange(t *testing.T) {
	report := SalesReport{From: swissDay(2026, 7, 24), To: swissDay(2026, 7, 25)}
	body, err := RenderSalesReport(report)
	if err != nil {
		t.Fatalf("render empty: %v", err)
	}
	f := openRendered(t, body)
	if got, _ := f.GetCellValue(salesSheetLines, "A1"); got != "Datum" {
		t.Fatalf("empty workbook still needs its headers, got %q", got)
	}
}

func TestSalesTitleSpansMultipleDays(t *testing.T) {
	report := testSalesReport()
	report.To = swissDay(2026, 7, 27)
	body, err := RenderSalesReport(report)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	f := openRendered(t, body)
	if got, _ := f.GetCellValue(salesSheetSummary, "A1"); got != "Verkäufe 24.07.2026 bis 26.07.2026" {
		t.Fatalf("title = %q", got)
	}
}
