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
// into the MJML <mj-table> rows. Children with a Price (deposits) render as an
// indented priced sub-line; those without (config/bundle options) are title-only.
type receiptChildView struct {
	Label string
	Price string
}

type receiptLineView struct {
	Label    string
	Price    string
	Children []receiptChildView
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

func renderReceiptHTML(ctx context.Context, b emailBranding, data ReceiptEmailData) (string, error) {
	items := make([]receiptLineView, 0, len(data.Items))
	for _, it := range data.Items {
		children := make([]receiptChildView, 0, len(it.Children))
		for _, c := range it.Children {
			cv := receiptChildView{Label: html.EscapeString(c.Title)}
			if c.Cents > 0 {
				cv.Price = formatCHF(c.Cents * int64(c.Quantity))
			}
			children = append(children, cv)
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
<mj-section padding="0 44px">
  <mj-column>
    <mj-text css-class="heading" font-size="23px" font-weight="700" color="#0a0a0a" line-height="1.3" padding="0 0 6px 0">Deine Quittung</mj-text>
    <mj-text css-class="body" color="#737373" line-height="1.6" padding="0 0 26px 0">Vielen Dank für deine Bestellung!</mj-text>

    <mj-table css-class="body" cellpadding="0" cellspacing="0" width="100%" font-size="14px" line-height="1.4">
      <tr>
        <td style="padding:7px 0;color:#737373;">Bestellnummer</td>
        <td align="right" style="padding:7px 0;color:#0a0a0a;">{{.OrderID | esc}}</td>
      </tr>
      <tr>
        <td style="padding:7px 0;color:#737373;">Datum</td>
        <td align="right" style="padding:7px 0;color:#0a0a0a;">{{.OrderDate | esc}}</td>
      </tr>
      <tr>
        <td style="padding:7px 0;color:#737373;">Zahlungsart</td>
        <td align="right" style="padding:7px 0;color:#0a0a0a;">{{.Method | esc}}</td>
      </tr>
    </mj-table>

    <mj-divider border-width="1px" border-color="#e5e5e5" padding="18px 0" />

    <mj-table css-class="body" cellpadding="0" cellspacing="0" width="100%" font-size="15px" line-height="1.4">
      {{range .Items}}
      <tr>
        <td style="padding:9px 0;color:#0a0a0a;">{{.Label}}</td>
        <td align="right" style="padding:9px 0;color:#0a0a0a;white-space:nowrap;">{{.Price}}</td>
      </tr>
      {{range .Children}}
      <tr>
        <td style="padding:0 0 7px 16px;color:#737373;font-size:13px;">{{.Label}}</td>
        <td align="right" style="padding:0 0 7px 0;color:#737373;font-size:13px;white-space:nowrap;">{{.Price}}</td>
      </tr>
      {{end}}
      {{end}}
    </mj-table>

    <mj-divider border-width="1px" border-color="#e5e5e5" padding="16px 0" />

    <mj-table css-class="heading" cellpadding="0" cellspacing="0" width="100%" font-size="16px">
      <tr>
        <td style="font-weight:700;color:#0a0a0a;">Total</td>
        <td align="right" style="font-weight:700;color:#0a0a0a;white-space:nowrap;">{{.Total}}</td>
      </tr>
    </mj-table>
    <mj-text css-class="body" font-size="12px" color="#737373" line-height="1.5" padding="8px 0 0 0">Alle Preise in CHF inkl. MwSt.</mj-text>

    <mj-button css-class="btn" width="100%" background-color="{{.Branding.ButtonBg}}" color="{{.Branding.ButtonText}}" border-radius="{{.Branding.ButtonRadius}}" font-size="15px" font-weight="600" inner-padding="16px 24px" padding="28px 0 0 0" href="{{.URL | esc}}">Bestellung &amp; QR-Code anzeigen</mj-button>
    <mj-text css-class="body" font-size="12px" color="#737373" line-height="1.5" padding="16px 0 0 0">Dies ist eine automatisch generierte Quittung. Bitte bewahre diese E-Mail als Zahlungsbeleg auf.</mj-text>
  </mj-column>
</mj-section>
{{end}}`
