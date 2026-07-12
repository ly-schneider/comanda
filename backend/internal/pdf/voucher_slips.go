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
	// BrandName and BrandIconPNG render the tenant's logo + name above the QR,
	// mirroring the auth screen and email headers. Either may be empty.
	BrandName    string
	BrandIconPNG []byte
}

const (
	VoucherDefaultCount = 5
	VoucherMinCount     = 1
	VoucherMaxCount     = 500

	// Fixed artist-badge credential size, so slips cut out to a consistent size.
	voucherBadgeWidthMM  = 76.2  // 3 in
	voucherBadgeHeightMM = 127.0 // 5 in
	voucherBadgePadMM    = 8.0

	voucherQRPixels     = 1024
	voucherBrandIconMM  = 12.0
	voucherBrandFont    = 10.0
	voucherBrandLH      = 4.5
	voucherGapBrandIcon = 2.0
	voucherGapBrand     = 4.0

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
	qrName      string
	brandLines  []string
	brandIcon   string
	brandAspect float64
	labelLines  []string
	catLines    []string
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
	if len(slip.BrandIconPNG) > 0 {
		if info := pdf.RegisterImageOptionsReader("vbrand", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(slip.BrandIconPNG)); info != nil && info.Height() > 0 {
			rv.brandIcon = "vbrand"
			rv.brandAspect = info.Width() / info.Height()
		}
	}
	if name := strings.TrimSpace(slip.BrandName); name != "" {
		rv.brandLines = wrapTextLines(tr(name), textWidth, voucherBrandFont, pdf, "B")
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

	innerX := x + voucherBadgePadMM
	innerW := voucherBadgeWidthMM - 2*voucherBadgePadMM

	contentH := brandHeaderHeight(v) + qrSize + voucherGapQR +
		float64(len(hintLines))*voucherHintLH + voucherGapLabel +
		float64(len(v.labelLines))*voucherLabelLH + voucherGapCat +
		float64(len(v.catLines))*voucherCatLH

	startY := y + (voucherBadgeHeightMM-contentH)/2
	if minY := y + voucherBadgePadMM; startY < minY {
		startY = minY
	}

	qrY := drawBrandHeader(pdf, x, startY, innerX, innerW, v)
	pdf.ImageOptions(v.qrName, x+(voucherBadgeWidthMM-qrSize)/2, qrY, qrSize, qrSize, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	textY := qrY + qrSize + voucherGapQR

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

// brandIconSize scales the icon to voucherBrandIconMM tall, capping its width to
// the inner column (a wide fallback logo shrinks instead of overflowing).
func brandIconSize(aspect float64) (w, h float64) {
	h = voucherBrandIconMM
	w = h * aspect
	if innerW := voucherBadgeWidthMM - 2*voucherBadgePadMM; w > innerW {
		w = innerW
		h = w / aspect
	}
	return
}

func brandHeaderHeight(v renderedVoucher) float64 {
	h := 0.0
	if v.brandIcon != "" {
		_, ih := brandIconSize(v.brandAspect)
		h += ih + voucherGapBrandIcon
	}
	h += float64(len(v.brandLines)) * voucherBrandLH
	if h > 0 {
		h += voucherGapBrand
	}
	return h
}

// drawBrandHeader renders the tenant icon + name at curY and returns the Y where
// the QR should start (unchanged when there is no branding).
func drawBrandHeader(pdf *fpdf.Fpdf, x, curY, innerX, innerW float64, v renderedVoucher) float64 {
	if v.brandIcon == "" && len(v.brandLines) == 0 {
		return curY
	}
	if v.brandIcon != "" {
		iconW, iconH := brandIconSize(v.brandAspect)
		pdf.ImageOptions(v.brandIcon, x+(voucherBadgeWidthMM-iconW)/2, curY, iconW, iconH, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
		curY += iconH + voucherGapBrandIcon
	}
	if len(v.brandLines) > 0 {
		pdf.SetFont("Helvetica", "B", voucherBrandFont)
		pdf.SetTextColor(20, 20, 20)
		for _, line := range v.brandLines {
			pdf.SetXY(innerX, curY)
			pdf.CellFormat(innerW, voucherBrandLH, line, "", 0, "C", false, 0, "")
			curY += voucherBrandLH
		}
	}
	return curY + voucherGapBrand
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
