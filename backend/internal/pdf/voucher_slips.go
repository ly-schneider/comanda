package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)

type VoucherSlip struct {
	Label      string
	Categories []string
	// QRBytes are the raw decoded token bytes (not the base64url string), rendered
	// byte-mode so the station reads back the exact bytes it verifies offline.
	QRBytes []byte
}

const (
	VoucherDefaultCols = 3
	VoucherMinCols     = 1
	VoucherMaxCols     = 6

	voucherQRSizeMM  = 42.0
	voucherQRPixels  = 1024
	voucherPaddingMM = 3.0
	voucherLabelFont = 10.0
	voucherLabelLH   = 4.0
	voucherCatFont   = 7.0
	voucherCatLH     = 3.0
	voucherHintFont  = 5.5
	voucherHintLH    = 2.3
	voucherHintText  = "An der Station vorzeigen"
	voucherCatPrefix = "Gültig für: "
	voucherEmptyText = "Keine aktiven Gutscheine zum Drucken."
)

type renderedVoucher struct {
	qrName     string
	labelLines []string
	catLines   []string
}

// An empty voucher list yields a placeholder page rather than an error, so an
// admin print with no active vouchers still opens a valid PDF.
func RenderVoucherSlips(vouchers []VoucherSlip, cols int) ([]byte, error) {
	if cols < VoucherMinCols {
		cols = VoucherDefaultCols
	}
	if cols > VoucherMaxCols {
		cols = VoucherMaxCols
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pageMarginMM, pageMarginMM, pageMarginMM)
	pdf.SetAutoPageBreak(false, pageMarginMM)
	pdf.SetTextColor(20, 20, 20)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	if len(vouchers) == 0 {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "I", 12)
		pdf.SetTextColor(110, 110, 110)
		pdf.SetXY(pageMarginMM, pageHeight/2)
		pdf.CellFormat(pageWidth-2*pageMarginMM, 8, tr(voucherEmptyText), "", 0, "C", false, 0, "")
		return output(pdf)
	}

	usableWidth := pageWidth - 2*pageMarginMM
	slipWidth := (usableWidth - float64(cols-1)*slipHGap) / float64(cols)
	textWidth := slipWidth - 2*voucherPaddingMM

	qrSize := voucherQRSizeMM
	if maxQR := textWidth; qrSize > maxQR {
		qrSize = maxQR
	}

	hintLines := wrapTextLines(tr(voucherHintText), textWidth, voucherHintFont, pdf, "I")

	rendered := make([]renderedVoucher, len(vouchers))
	maxLabelLines, maxCatLines := 1, 1
	for i, v := range vouchers {
		png, err := qrcode.Encode(string(v.QRBytes), qrcode.Medium, voucherQRPixels)
		if err != nil {
			return nil, fmt.Errorf("encode voucher qr: %w", err)
		}
		name := fmt.Sprintf("vqr%d", i)
		pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(png))

		labelLines := wrapTextLines(tr(v.Label), textWidth, voucherLabelFont, pdf, "B")
		catLines := wrapTextLines(tr(voucherCatPrefix+strings.Join(v.Categories, ", ")), textWidth, voucherCatFont, pdf, "")
		if len(labelLines) > maxLabelLines {
			maxLabelLines = len(labelLines)
		}
		if len(catLines) > maxCatLines {
			maxCatLines = len(catLines)
		}
		rendered[i] = renderedVoucher{qrName: name, labelLines: labelLines, catLines: catLines}
	}

	slipHeight := voucherPaddingMM + qrSize + 1.2 +
		float64(len(hintLines))*voucherHintLH + 1.5 +
		float64(maxLabelLines)*voucherLabelLH + 1.2 +
		float64(maxCatLines)*voucherCatLH + voucherPaddingMM

	maxRows := int((pageHeight - 2*pageMarginMM + slipVGap) / (slipHeight + slipVGap))
	if maxRows < 1 {
		maxRows = 1
	}
	perPage := maxRows * cols

	idx := 0
	for idx < len(rendered) {
		pdf.AddPage()
		pageSlips := perPage
		if remaining := len(rendered) - idx; remaining < pageSlips {
			pageSlips = remaining
		}
		for slot := 0; slot < pageSlips; slot++ {
			row := slot / cols
			col := slot % cols
			x := pageMarginMM + float64(col)*(slipWidth+slipHGap)
			y := pageMarginMM + float64(row)*(slipHeight+slipVGap)
			drawVoucherSlip(pdf, x, y, slipWidth, qrSize, rendered[idx+slot], hintLines)
		}
		drawVoucherCutLines(pdf, slipWidth, slipHeight, cols, pageSlips)
		idx += pageSlips
	}

	return output(pdf)
}

func drawVoucherSlip(pdf *fpdf.Fpdf, x, y, w, qrSize float64, v renderedVoucher, hintLines []string) {
	qrX := x + (w-qrSize)/2
	qrY := y + voucherPaddingMM
	pdf.ImageOptions(v.qrName, qrX, qrY, qrSize, qrSize, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")

	textY := qrY + qrSize + 1.2
	pdf.SetFont("Helvetica", "I", voucherHintFont)
	pdf.SetTextColor(110, 110, 110)
	for _, line := range hintLines {
		pdf.SetXY(x+voucherPaddingMM, textY)
		pdf.CellFormat(w-2*voucherPaddingMM, voucherHintLH, line, "", 0, "C", false, 0, "")
		textY += voucherHintLH
	}

	textY += 1.5
	pdf.SetFont("Helvetica", "B", voucherLabelFont)
	pdf.SetTextColor(20, 20, 20)
	for _, line := range v.labelLines {
		pdf.SetXY(x+voucherPaddingMM, textY)
		pdf.CellFormat(w-2*voucherPaddingMM, voucherLabelLH, line, "", 0, "C", false, 0, "")
		textY += voucherLabelLH
	}

	textY += 1.2
	pdf.SetFont("Helvetica", "", voucherCatFont)
	pdf.SetTextColor(90, 90, 90)
	for _, line := range v.catLines {
		pdf.SetXY(x+voucherPaddingMM, textY)
		pdf.CellFormat(w-2*voucherPaddingMM, voucherCatLH, line, "", 0, "C", false, 0, "")
		textY += voucherCatLH
	}
	pdf.SetTextColor(20, 20, 20)
}

func drawVoucherCutLines(pdf *fpdf.Fpdf, slipWidth, slipHeight float64, cols, slipsOnPage int) {
	pdf.SetDrawColor(160, 160, 160)
	pdf.SetLineWidth(0.1)
	pdf.SetDashPattern([]float64{0.8, 0.8}, 0)
	defer pdf.SetDashPattern([]float64{}, 0)

	rows := (slipsOnPage-1)/cols + 1
	topEdge := pageMarginMM - slipVGap/2
	bottomEdge := pageMarginMM + float64(rows-1)*(slipHeight+slipVGap) + slipHeight + slipVGap/2

	for r := 0; r <= rows; r++ {
		var yy float64
		switch r {
		case 0:
			yy = topEdge
		case rows:
			yy = bottomEdge
		default:
			yy = pageMarginMM + float64(r)*(slipHeight+slipVGap) - slipVGap/2
		}
		pdf.Line(pageMarginMM-1, yy, pageWidth-pageMarginMM+1, yy)
	}
	for c := 1; c < cols; c++ {
		xx := pageMarginMM + float64(c)*slipWidth + float64(c-1)*slipHGap + slipHGap/2
		pdf.Line(xx, topEdge, xx, bottomEdge)
	}
}

func output(pdf *fpdf.Fpdf) ([]byte, error) {
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write pdf: %w", err)
	}
	return buf.Bytes(), nil
}
