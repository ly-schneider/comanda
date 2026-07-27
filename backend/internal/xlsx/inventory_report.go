package xlsx

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

const (
	sheetSummary   = "Übersicht"
	sheetMovements = "Bewegungen"
)

// Bounds arrive as UTC instants derived from local midnight, so every rendered
// date has to be read back in wall-clock time or it lands a day early.
var reportLocation = loadReportLocation()

func loadReportLocation() *time.Location {
	loc, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		return time.UTC
	}
	return loc
}

// Location is the timezone every date in the workbook is rendered in.
func Location() *time.Location { return reportLocation }

type SummaryRow struct {
	Category    string
	Subcategory string
	Product     string
	Variant     string
	Opening     int
	Sold        int
	Refunded    int
	Cancelled   int
	Manual      int
	Closing     int
}

type MovementRow struct {
	At          time.Time
	Category    string
	Subcategory string
	Product     string
	Variant     string
	Delta       int
	Reason      string
	OrderID     string
	CreatedBy   string
	CreatedByID string
}

type InventoryReport struct {
	ScopeLabel string
	From       time.Time
	To         time.Time
	Summary    []SummaryRow
	Movements  []MovementRow
}

var summaryHeaders = []string{
	"Kategorie", "Unterkategorie", "Produkt", "Variante",
	"Anfangsbestand", "Verkauft", "Erstattet", "Storniert", "Manuell", "Endbestand",
}

var movementHeaders = []string{
	"Datum", "Kategorie", "Unterkategorie", "Produkt", "Variante",
	"Delta", "Grund", "Bestellung", "Erfasst von",
}

func RenderInventoryReport(report InventoryReport) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EFEFEF"}},
	})
	if err != nil {
		return nil, err
	}
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("dd.MM.yyyy HH:mm")})
	if err != nil {
		return nil, err
	}

	if err := f.SetSheetName(f.GetSheetName(0), sheetSummary); err != nil {
		return nil, err
	}
	if _, err := f.NewSheet(sheetMovements); err != nil {
		return nil, err
	}

	if err := writeSummary(f, report, headerStyle); err != nil {
		return nil, err
	}
	if err := writeMovements(f, report, headerStyle, dateStyle); err != nil {
		return nil, err
	}

	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeSummary(f *excelize.File, report InventoryReport, headerStyle int) error {
	if err := setRow(f, sheetSummary, 1, []any{title(report)}); err != nil {
		return err
	}
	if err := setRow(f, sheetSummary, 2, []any{"Bundle-Verkäufe erscheinen bei den enthaltenen Produkten."}); err != nil {
		return err
	}

	const headerRow = 4
	if err := setRow(f, sheetSummary, headerRow, toAny(summaryHeaders)); err != nil {
		return err
	}
	if err := f.SetCellStyle(sheetSummary, cell(1, headerRow), cell(len(summaryHeaders), headerRow), headerStyle); err != nil {
		return err
	}

	for i, row := range report.Summary {
		values := []any{
			row.Category, row.Subcategory, row.Product, row.Variant,
			row.Opening, row.Sold, row.Refunded, row.Cancelled, row.Manual, row.Closing,
		}
		if err := setRow(f, sheetSummary, headerRow+1+i, values); err != nil {
			return err
		}
	}

	if err := f.SetPanes(sheetSummary, &excelize.Panes{
		Freeze: true, Split: false, YSplit: headerRow, TopLeftCell: cell(1, headerRow+1), ActivePane: "bottomLeft",
	}); err != nil {
		return err
	}
	if len(report.Summary) > 0 {
		if err := f.AutoFilter(sheetSummary, fmt.Sprintf("%s:%s", cell(1, headerRow), cell(len(summaryHeaders), headerRow+len(report.Summary))), nil); err != nil {
			return err
		}
	}
	return setColWidths(f, sheetSummary, []float64{18, 18, 26, 14, 16, 12, 12, 12, 12, 14})
}

func writeMovements(f *excelize.File, report InventoryReport, headerStyle, dateStyle int) error {
	const headerRow = 1
	if err := setRow(f, sheetMovements, headerRow, toAny(movementHeaders)); err != nil {
		return err
	}
	if err := f.SetCellStyle(sheetMovements, cell(1, headerRow), cell(len(movementHeaders), headerRow), headerStyle); err != nil {
		return err
	}

	for i, row := range report.Movements {
		r := headerRow + 1 + i
		values := []any{
			row.At.In(reportLocation), row.Category, row.Subcategory, row.Product, row.Variant,
			row.Delta, row.Reason, row.OrderID, row.CreatedBy,
		}
		if err := setRow(f, sheetMovements, r, values); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheetMovements, cell(1, r), cell(1, r), dateStyle); err != nil {
			return err
		}
	}

	if err := f.SetPanes(sheetMovements, &excelize.Panes{
		Freeze: true, Split: false, YSplit: headerRow, TopLeftCell: cell(1, headerRow+1), ActivePane: "bottomLeft",
	}); err != nil {
		return err
	}
	if len(report.Movements) > 0 {
		if err := f.AutoFilter(sheetMovements, fmt.Sprintf("%s:%s", cell(1, headerRow), cell(len(movementHeaders), headerRow+len(report.Movements))), nil); err != nil {
			return err
		}
	}
	return setColWidths(f, sheetMovements, []float64{18, 18, 18, 26, 14, 10, 20, 16, 20})
}

func title(report InventoryReport) string {
	return fmt.Sprintf("Inventarverlauf %s · %s bis %s",
		report.ScopeLabel,
		report.From.In(reportLocation).Format("02.01.2006"),
		// [from, to): the last included day precedes the exclusive bound.
		report.To.Add(-time.Nanosecond).In(reportLocation).Format("02.01.2006"),
	)
}

func setRow(f *excelize.File, sheet string, row int, values []any) error {
	return f.SetSheetRow(sheet, cell(1, row), &values)
}

func setColWidths(f *excelize.File, sheet string, widths []float64) error {
	for i, w := range widths {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return err
		}
	}
	return nil
}

func cell(col, row int) string {
	name, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return "A1"
	}
	return name
}

func toAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func strPtr(s string) *string { return &s }
