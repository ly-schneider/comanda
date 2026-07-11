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
	VoucherDefaultCount = 5
	VoucherMinCount     = 1
	VoucherMaxCount     = 500

	// Fixed artist-badge credential size, so slips cut out to a consistent size.
	voucherBadgeWidthMM  = 76.2  // 3 in
	voucherBadgeHeightMM = 127.0 // 5 in
	voucherBadgePadMM    = 8.0

	voucherQRPixels  = 1024
	voucherLabelFont = 11.0
	voucherLabelLH   = 4.5
	voucherCatFont   = 8.0
	voucherCatLH     = 3.4
	voucherHintFont  = 6.0
	voucherHintLH    = 2.6
	voucherHintText  = "An der Station vorzeigen"
	voucherCatPrefix = "Gültig für: "

	voucherGapQR    = 2.5
	voucherGapLabel = 2.0
	voucherGapCat   = 1.5
)

type renderedVoucher struct {
	qrName     string
	labelLines []string
	catLines   []string
}

// RenderVoucherSlips prints count identical badge-sized copies of one voucher.
func RenderVoucherSlips(slip VoucherSlip, count int) ([]byte, error) {
	if count < VoucherMinCount {
		count = VoucherDefaultCount
	}
	if count > VoucherMaxCount {
		count = VoucherMaxCount
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pageMarginMM, pageMarginMM, pageMarginMM)
	pdf.SetAutoPageBreak(false, pageMarginMM)
	pdf.SetTextColor(20, 20, 20)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	png, err := qrcode.Encode(string(slip.QRBytes), qrcode.Medium, voucherQRPixels)
	if err != nil {
		return nil, fmt.Errorf("encode voucher qr: %w", err)
	}
	pdf.RegisterImageOptionsReader("vqr", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(png))

	textWidth := voucherBadgeWidthMM - 2*voucherBadgePadMM
	qrSize := textWidth
	rv := renderedVoucher{
		qrName:     "vqr",
		labelLines: wrapTextLines(tr(slip.Label), textWidth, voucherLabelFont, pdf, "B"),
		catLines:   wrapTextLines(tr(voucherCatPrefix+strings.Join(slip.Categories, ", ")), textWidth, voucherCatFont, pdf, ""),
	}
	hintLines := wrapTextLines(tr(voucherHintText), textWidth, voucherHintFont, pdf, "I")

	cellW := voucherBadgeWidthMM + slipHGap
	cellH := voucherBadgeHeightMM + slipVGap
	cols := int((pageWidth - 2*pageMarginMM + slipHGap) / cellW)
	rows := int((pageHeight - 2*pageMarginMM + slipVGap) / cellH)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	perPage := cols * rows

	gridW := float64(cols)*voucherBadgeWidthMM + float64(cols-1)*slipHGap
	gridH := float64(rows)*voucherBadgeHeightMM + float64(rows-1)*slipVGap
	offsetX := (pageWidth - gridW) / 2
	offsetY := (pageHeight - gridH) / 2

	for printed := 0; printed < count; {
		pdf.AddPage()
		pageSlips := perPage
		if remaining := count - printed; remaining < pageSlips {
			pageSlips = remaining
		}
		for slot := 0; slot < pageSlips; slot++ {
			x := offsetX + float64(slot%cols)*(voucherBadgeWidthMM+slipHGap)
			y := offsetY + float64(slot/cols)*(voucherBadgeHeightMM+slipVGap)
			drawVoucherBadge(pdf, x, y, qrSize, rv, hintLines)
		}
		printed += pageSlips
	}

	return output(pdf)
}

func drawVoucherBadge(pdf *fpdf.Fpdf, x, y, qrSize float64, v renderedVoucher, hintLines []string) {
	drawVoucherBorder(pdf, x, y)

	contentH := qrSize + voucherGapQR +
		float64(len(hintLines))*voucherHintLH + voucherGapLabel +
		float64(len(v.labelLines))*voucherLabelLH + voucherGapCat +
		float64(len(v.catLines))*voucherCatLH

	startY := y + (voucherBadgeHeightMM-contentH)/2
	if minY := y + voucherBadgePadMM; startY < minY {
		startY = minY
	}

	innerX := x + voucherBadgePadMM
	innerW := voucherBadgeWidthMM - 2*voucherBadgePadMM

	pdf.ImageOptions(v.qrName, x+(voucherBadgeWidthMM-qrSize)/2, startY, qrSize, qrSize, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	textY := startY + qrSize + voucherGapQR

	pdf.SetFont("Helvetica", "I", voucherHintFont)
	pdf.SetTextColor(110, 110, 110)
	for _, line := range hintLines {
		pdf.SetXY(innerX, textY)
		pdf.CellFormat(innerW, voucherHintLH, line, "", 0, "C", false, 0, "")
		textY += voucherHintLH
	}

	textY += voucherGapLabel
	pdf.SetFont("Helvetica", "B", voucherLabelFont)
	pdf.SetTextColor(20, 20, 20)
	for _, line := range v.labelLines {
		pdf.SetXY(innerX, textY)
		pdf.CellFormat(innerW, voucherLabelLH, line, "", 0, "C", false, 0, "")
		textY += voucherLabelLH
	}

	textY += voucherGapCat
	pdf.SetFont("Helvetica", "", voucherCatFont)
	pdf.SetTextColor(90, 90, 90)
	for _, line := range v.catLines {
		pdf.SetXY(innerX, textY)
		pdf.CellFormat(innerW, voucherCatLH, line, "", 0, "C", false, 0, "")
		textY += voucherCatLH
	}
	pdf.SetTextColor(20, 20, 20)
}

func drawVoucherBorder(pdf *fpdf.Fpdf, x, y float64) {
	pdf.SetDrawColor(160, 160, 160)
	pdf.SetLineWidth(0.2)
	pdf.SetDashPattern([]float64{1.2, 1.0}, 0)
	pdf.Rect(x, y, voucherBadgeWidthMM, voucherBadgeHeightMM, "D")
	pdf.SetDashPattern([]float64{}, 0)
}

func output(pdf *fpdf.Fpdf) ([]byte, error) {
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write pdf: %w", err)
	}
	return buf.Bytes(), nil
}
