package xlsx

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

const (
	salesSheetSummary  = "Zusammenfassung"
	salesSheetArticles = "Pro Artikel"
	salesSheetLines    = "Verkäufe"
	salesSheetOrders   = "Bestellungen"
)

// SaleRow is one article handed over in one order. A bundle contributes one row
// per component, never a row of its own, so the rows sum to the takings.
type SaleRow struct {
	At            time.Time
	OrderID       string
	Channel       string
	PaymentMethod string
	Category      string
	Subcategory   string
	Product       string
	Variant       string
	Source        string
	FromBundle    bool
	Quantity      int
	RefundedQty   int
	UnitCents     int64
	RevenueCents  int64
	DepositCents  int64
	RefundedCents int64
	GratisCents   int64
}

// ArticleRow aggregates SaleRows per product and variant.
type ArticleRow struct {
	Category      string
	Subcategory   string
	Product       string
	Variant       string
	Quantity      int
	BundleQty     int
	GratisQty     int
	RefundedQty   int
	RevenueCents  int64
	DepositCents  int64
	RefundedCents int64
	GratisCents   int64
}

// OrderRow is the receipt-level view: what a single order took in.
type OrderRow struct {
	At            time.Time
	OrderID       string
	Channel       string
	Status        string
	PaymentMethod string
	ItemCount     int
	TotalCents    int64
	RefundedCents int64
	Contact       string
}

// MethodRow totals one payment method for the cash-up.
type MethodRow struct {
	Method        string
	OrderCount    int
	RevenueCents  int64
	DepositCents  int64
	RefundedCents int64
	GratisCents   int64
}

type SalesTotals struct {
	OrderCount       int
	ItemCount        int
	RevenueCents     int64
	DepositCents     int64
	RefundedCents    int64
	UnlinkedRefunds  int64
	GratisCents      int64
	GratisItemCount  int
	GratisOrderCount int
}

type SalesReport struct {
	From     time.Time
	To       time.Time
	Totals   SalesTotals
	Methods  []MethodRow
	Articles []ArticleRow
	Sales    []SaleRow
	Orders   []OrderRow
}

func (r SalesReport) NetCents() int64 {
	return r.Totals.RevenueCents + r.Totals.DepositCents - r.Totals.RefundedCents
}

func (m MethodRow) NetCents() int64 {
	return m.RevenueCents + m.DepositCents - m.RefundedCents
}

func (a ArticleRow) NetCents() int64 {
	return a.RevenueCents + a.DepositCents - a.RefundedCents
}

func (s SaleRow) NetCents() int64 {
	return s.RevenueCents + s.DepositCents - s.RefundedCents
}

var salesHeaders = []string{
	"Datum", "Bestellung", "Kanal", "Zahlungsart", "Kategorie", "Unterkategorie",
	"Artikel", "Variante", "Herkunft", "Menge", "Menge erstattet", "Einzelpreis",
	"Warenumsatz", "Depot", "Erstattet", "Netto", "Gratis-Wert",
}

var articleHeaders = []string{
	"Kategorie", "Unterkategorie", "Artikel", "Variante", "Menge",
	"davon über Bundle", "davon gratis", "Menge erstattet", "Warenumsatz",
	"Depot", "Erstattet", "Netto", "Ø Preis",
}

var orderHeaders = []string{
	"Datum", "Bestellung", "Kanal", "Status", "Zahlungsart", "Artikel",
	"Total", "Erstattet", "Netto", "Kontakt",
}

var methodHeaders = []string{
	"Zahlungsart", "Bestellungen", "Warenumsatz", "Depot", "Erstattet", "Netto",
}

type salesStyles struct {
	header int
	title  int
	money  int
	date   int
	label  int
}

func RenderSalesReport(report SalesReport) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	styles, err := newSalesStyles(f)
	if err != nil {
		return nil, err
	}

	if err := f.SetSheetName(f.GetSheetName(0), salesSheetSummary); err != nil {
		return nil, err
	}
	for _, name := range []string{salesSheetArticles, salesSheetLines, salesSheetOrders} {
		if _, err := f.NewSheet(name); err != nil {
			return nil, err
		}
	}

	if err := writeSalesSummary(f, report, styles); err != nil {
		return nil, err
	}
	if err := writeArticles(f, report, styles); err != nil {
		return nil, err
	}
	if err := writeSales(f, report, styles); err != nil {
		return nil, err
	}
	if err := writeOrders(f, report, styles); err != nil {
		return nil, err
	}

	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func newSalesStyles(f *excelize.File) (salesStyles, error) {
	var s salesStyles
	var err error
	if s.header, err = f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EFEFEF"}},
	}); err != nil {
		return s, err
	}
	if s.title, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 13}}); err != nil {
		return s, err
	}
	// Amounts are real numbers, not text: the bookkeeper has to be able to sum a
	// column without retyping it.
	if s.money, err = f.NewStyle(&excelize.Style{CustomNumFmt: strPtr(`#,##0.00`)}); err != nil {
		return s, err
	}
	if s.date, err = f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("dd.MM.yyyy HH:mm")}); err != nil {
		return s, err
	}
	if s.label, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err != nil {
		return s, err
	}
	return s, nil
}

func writeSalesSummary(f *excelize.File, report SalesReport, styles salesStyles) error {
	if err := setRow(f, salesSheetSummary, 1, []any{salesTitle(report)}); err != nil {
		return err
	}
	if err := f.SetCellStyle(salesSheetSummary, cell(1, 1), cell(1, 1), styles.title); err != nil {
		return err
	}
	if err := setRow(f, salesSheetSummary, 2, []any{"Alle Beträge in CHF. Gratis-Bestellungen zählen als Umsatz 0 — ihr Warenwert steht separat unter «Gratis abgegeben»."}); err != nil {
		return err
	}

	t := report.Totals
	rows := []struct {
		label string
		value any
		money bool
	}{
		{"Bestellungen", t.OrderCount, false},
		{"Verkaufte Artikel", t.ItemCount, false},
		{"Warenumsatz", chf(t.RevenueCents), true},
		{"Depot kassiert", chf(t.DepositCents), true},
		{"Erstattungen", chf(-t.RefundedCents), true},
		{"Nettoeinnahmen", chf(report.NetCents()), true},
		{"Gratis abgegeben (Warenwert)", chf(t.GratisCents), true},
	}

	row := 4
	for _, r := range rows {
		if err := setRow(f, salesSheetSummary, row, []any{r.label, r.value}); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetSummary, cell(1, row), cell(1, row), styles.label); err != nil {
			return err
		}
		if r.money {
			if err := f.SetCellStyle(salesSheetSummary, cell(2, row), cell(2, row), styles.money); err != nil {
				return err
			}
		}
		row++
	}

	if t.UnlinkedRefunds != 0 {
		if err := setRow(f, salesSheetSummary, row, []any{
			"davon Erstattungen ohne Artikelbezug", chf(-t.UnlinkedRefunds),
		}); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetSummary, cell(2, row), cell(2, row), styles.money); err != nil {
			return err
		}
		row++
	}
	if t.GratisOrderCount > 0 {
		if err := setRow(f, salesSheetSummary, row, []any{
			"Gratis-Bestellungen", t.GratisOrderCount, "Artikel", t.GratisItemCount,
		}); err != nil {
			return err
		}
		row++
	}

	row += 2
	if err := setRow(f, salesSheetSummary, row, []any{"Nach Zahlungsart"}); err != nil {
		return err
	}
	if err := f.SetCellStyle(salesSheetSummary, cell(1, row), cell(1, row), styles.title); err != nil {
		return err
	}
	row++

	headerRow := row
	if err := setRow(f, salesSheetSummary, headerRow, toAny(methodHeaders)); err != nil {
		return err
	}
	if err := f.SetCellStyle(salesSheetSummary, cell(1, headerRow), cell(len(methodHeaders), headerRow), styles.header); err != nil {
		return err
	}
	row++

	for _, m := range report.Methods {
		values := []any{
			m.Method, m.OrderCount,
			chf(m.RevenueCents), chf(m.DepositCents), chf(-m.RefundedCents), chf(m.NetCents()),
		}
		if err := setRow(f, salesSheetSummary, row, values); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetSummary, cell(3, row), cell(6, row), styles.money); err != nil {
			return err
		}
		row++
	}

	return setColWidths(f, salesSheetSummary, []float64{34, 16, 16, 14, 14, 14})
}

func writeArticles(f *excelize.File, report SalesReport, styles salesStyles) error {
	if err := setRow(f, salesSheetArticles, 1, []any{salesTitle(report)}); err != nil {
		return err
	}
	if err := f.SetCellStyle(salesSheetArticles, cell(1, 1), cell(1, 1), styles.title); err != nil {
		return err
	}
	if err := setRow(f, salesSheetArticles, 2, []any{"Artikel aus Bundles zählen beim enthaltenen Produkt; ihr Anteil am Bundle-Preis ist im Warenumsatz enthalten."}); err != nil {
		return err
	}

	const headerRow = 4
	if err := setRow(f, salesSheetArticles, headerRow, toAny(articleHeaders)); err != nil {
		return err
	}
	if err := f.SetCellStyle(salesSheetArticles, cell(1, headerRow), cell(len(articleHeaders), headerRow), styles.header); err != nil {
		return err
	}

	for i, a := range report.Articles {
		r := headerRow + 1 + i
		values := []any{
			a.Category, a.Subcategory, a.Product, a.Variant,
			a.Quantity, a.BundleQty, a.GratisQty, a.RefundedQty,
			chf(a.RevenueCents), chf(a.DepositCents), chf(-a.RefundedCents), chf(a.NetCents()),
			chf(averageUnitCents(a)),
		}
		if err := setRow(f, salesSheetArticles, r, values); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetArticles, cell(9, r), cell(13, r), styles.money); err != nil {
			return err
		}
	}

	if err := freezeAndFilter(f, salesSheetArticles, headerRow, len(articleHeaders), len(report.Articles)); err != nil {
		return err
	}
	return setColWidths(f, salesSheetArticles, []float64{18, 18, 28, 12, 10, 17, 13, 15, 14, 12, 12, 12, 12})
}

func writeSales(f *excelize.File, report SalesReport, styles salesStyles) error {
	const headerRow = 1
	if err := setRow(f, salesSheetLines, headerRow, toAny(salesHeaders)); err != nil {
		return err
	}
	if err := f.SetCellStyle(salesSheetLines, cell(1, headerRow), cell(len(salesHeaders), headerRow), styles.header); err != nil {
		return err
	}

	for i, s := range report.Sales {
		r := headerRow + 1 + i
		values := []any{
			s.At.In(reportLocation), s.OrderID, s.Channel, s.PaymentMethod,
			s.Category, s.Subcategory, s.Product, s.Variant, s.Source,
			s.Quantity, s.RefundedQty, chf(s.UnitCents), chf(s.RevenueCents),
			chf(s.DepositCents), chf(-s.RefundedCents), chf(s.NetCents()), chf(s.GratisCents),
		}
		if err := setRow(f, salesSheetLines, r, values); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetLines, cell(1, r), cell(1, r), styles.date); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetLines, cell(12, r), cell(17, r), styles.money); err != nil {
			return err
		}
	}

	if err := freezeAndFilter(f, salesSheetLines, headerRow, len(salesHeaders), len(report.Sales)); err != nil {
		return err
	}
	return setColWidths(f, salesSheetLines, []float64{18, 14, 10, 18, 18, 18, 28, 12, 24, 8, 15, 12, 13, 10, 11, 11, 12})
}

func writeOrders(f *excelize.File, report SalesReport, styles salesStyles) error {
	const headerRow = 1
	if err := setRow(f, salesSheetOrders, headerRow, toAny(orderHeaders)); err != nil {
		return err
	}
	if err := f.SetCellStyle(salesSheetOrders, cell(1, headerRow), cell(len(orderHeaders), headerRow), styles.header); err != nil {
		return err
	}

	for i, o := range report.Orders {
		r := headerRow + 1 + i
		values := []any{
			o.At.In(reportLocation), o.OrderID, o.Channel, o.Status, o.PaymentMethod,
			o.ItemCount, chf(o.TotalCents), chf(-o.RefundedCents), chf(o.TotalCents - o.RefundedCents),
			o.Contact,
		}
		if err := setRow(f, salesSheetOrders, r, values); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetOrders, cell(1, r), cell(1, r), styles.date); err != nil {
			return err
		}
		if err := f.SetCellStyle(salesSheetOrders, cell(7, r), cell(9, r), styles.money); err != nil {
			return err
		}
	}

	if err := freezeAndFilter(f, salesSheetOrders, headerRow, len(orderHeaders), len(report.Orders)); err != nil {
		return err
	}
	return setColWidths(f, salesSheetOrders, []float64{18, 14, 10, 16, 18, 10, 12, 12, 12, 28})
}

func freezeAndFilter(f *excelize.File, sheet string, headerRow, cols, rows int) error {
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, Split: false, YSplit: headerRow,
		TopLeftCell: cell(1, headerRow+1), ActivePane: "bottomLeft",
	}); err != nil {
		return err
	}
	if rows == 0 {
		return nil
	}
	return f.AutoFilter(sheet, fmt.Sprintf("%s:%s", cell(1, headerRow), cell(cols, headerRow+rows)), nil)
}

func averageUnitCents(a ArticleRow) int64 {
	paidQty := a.Quantity - a.GratisQty
	if paidQty <= 0 {
		return 0
	}
	return a.RevenueCents / int64(paidQty)
}

func chf(cents int64) float64 {
	return float64(cents) / 100
}

func salesTitle(report SalesReport) string {
	return fmt.Sprintf("Verkäufe %s bis %s",
		report.From.In(reportLocation).Format("02.01.2006"),
		// [from, to): the last included day precedes the exclusive bound.
		report.To.Add(-time.Nanosecond).In(reportLocation).Format("02.01.2006"),
	)
}
