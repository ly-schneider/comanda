package service

import (
	"context"
	"fmt"
	"html"
	"time"
)

type ReceiptLineItem struct {
	Title    string
	Quantity int
	Cents    int64
	Children []ReceiptLineItem
}

type ReceiptEmailData struct {
	OrderID    string
	OrderURL   string
	OrderDate  string
	Items      []ReceiptLineItem
	TotalCents int64
	Method     string
}

func formatCHF(cents int64) string {
	whole := cents / 100
	frac := cents % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("CHF %d.%02d", whole, frac)
}

func formatOrderDate(t time.Time) string {
	loc, _ := time.LoadLocation("Europe/Zurich")
	t = t.In(loc)
	months := []string{
		"Januar", "Februar", "März", "April", "Mai", "Juni",
		"Juli", "August", "September", "Oktober", "November", "Dezember",
	}
	return fmt.Sprintf("%d. %s %d, %02d:%02d", t.Day(), months[t.Month()-1], t.Year(), t.Hour(), t.Minute())
}

// receiptLineView holds the pre-formatted, pre-escaped fields embedded directly
// into the MJML <mj-table> rows.
type receiptLineView struct {
	Label    string
	Price    string
	Children []string
}

type receiptView struct {
	baseView
	OrderID   string
	OrderDate string
	Method    string
	Items     []receiptLineView
	Total     string
	URL       string
}

// renderReceiptHTML renders the order receipt email to HTML.
func renderReceiptHTML(ctx context.Context, b emailBranding, data ReceiptEmailData) (string, error) {
	items := make([]receiptLineView, 0, len(data.Items))
	for _, it := range data.Items {
		children := make([]string, 0, len(it.Children))
		for _, c := range it.Children {
			children = append(children, html.EscapeString(c.Title))
		}
		items = append(items, receiptLineView{
			Label:    fmt.Sprintf("%s &times; %d", html.EscapeString(it.Title), it.Quantity),
			Price:    formatCHF(it.Cents * int64(it.Quantity)),
			Children: children,
		})
	}

	view := receiptView{
		baseView:  newBaseView("Deine Quittung von "+b.Name, "Vielen Dank für deine Bestellung bei "+b.Name+".", b),
		OrderID:   data.OrderID,
		OrderDate: data.OrderDate,
		Method:    data.Method,
		Items:     items,
		Total:     formatCHF(data.TotalCents),
		URL:       data.OrderURL,
	}
	return renderMJML(ctx, receiptTmpl, view)
}

const receiptContentMJML = `{{define "content"}}
<mj-section padding="26px 24px 2px 24px">
  <mj-column>
    <mj-text css-class="heading" font-size="21px" font-weight="700" color="#1A1A1A" line-height="1.3" padding="0 0 6px 0">Deine Quittung</mj-text>
    <mj-text css-class="body" color="#6E6E6E" line-height="1.6">Vielen Dank für deine Bestellung!</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="14px 24px 4px 24px">
  <mj-column css-class="panel" background-color="#F4F3F1" border="1px solid #E6E4E2" border-radius="10px" padding="14px 16px">
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.5">Bestellnr.</mj-text>
    <mj-text css-class="muted" font-family="{{.MonoStack}}" font-size="11px" color="#9A9A9A" padding="2px 0 8px 0">{{.OrderID | esc}}</mj-text>
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.6">Datum: {{.OrderDate | esc}}</mj-text>
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.6">Zahlungsart: {{.Method | esc}}</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="12px 24px 0 24px">
  <mj-column>
    <mj-table css-class="body" cellpadding="0" cellspacing="0" width="100%" font-size="14px" color="#1A1A1A" line-height="1.4">
      {{range .Items}}
      <tr>
        <td style="padding:8px 0;border-bottom:1px solid #EDEBE9;color:#1A1A1A;">{{.Label}}</td>
        <td align="right" style="padding:8px 0;border-bottom:1px solid #EDEBE9;color:#1A1A1A;white-space:nowrap;">{{.Price}}</td>
      </tr>
      {{range .Children}}
      <tr>
        <td colspan="2" style="padding:2px 0 2px 14px;border-bottom:1px solid #EDEBE9;color:#9A9A9A;font-size:12px;">{{.}}</td>
      </tr>
      {{end}}
      {{end}}
      <tr>
        <td style="padding:12px 0 0 0;font-weight:700;color:#1A1A1A;">Total</td>
        <td align="right" style="padding:12px 0 0 0;font-weight:700;color:#1A1A1A;white-space:nowrap;">{{.Total}}</td>
      </tr>
    </mj-table>
  </mj-column>
</mj-section>
<mj-section padding="6px 24px 0 24px">
  <mj-column>
    <mj-text css-class="muted" font-size="11px" color="#9A9A9A" line-height="1.5">Alle Preise in CHF inkl. MwSt.</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="18px 24px 26px 24px">
  <mj-column>
    <mj-button css-class="btn" width="100%" background-color="#1A1A1A" color="#FAFAFA" border-radius="10px" font-size="15px" font-weight="600" inner-padding="16px 24px" href="{{.URL | esc}}">Bestellung &amp; QR-Code anzeigen</mj-button>
    <mj-text css-class="muted" font-size="11px" color="#9A9A9A" line-height="1.5" padding="14px 0 0 0">Dies ist eine automatisch generierte Quittung. Bitte bewahre diese E-Mail als Zahlungsbeleg auf.</mj-text>
  </mj-column>
</mj-section>
{{end}}`
