package service

import (
	"strings"
	"testing"
)

// A tenant-controlled display name (settings.display_name) flows into Brand, so
// it must never reach email HTML unescaped.
func TestEmailTemplatesEscapeBrand(t *testing.T) {
	const payload = `<img src=x onerror=alert(1)>`
	const escaped = "&lt;img src=x onerror=alert(1)&gt;"

	cases := map[string]string{
		"invite":  renderInviteHTML(InviteEmailData{Brand: payload, InviteURL: "https://x.test/i", ExpiresAt: "soon"}),
		"otp":     renderOTPHTML(OTPEmailData{Brand: payload, Code: "123456", CodeTTL: "5 Minuten", SupportNote: "x"}),
		"receipt": renderReceiptHTML(ReceiptEmailData{Brand: payload, OrderID: "o1", OrderURL: "https://x.test/o", OrderDate: "today", Method: "card", TotalCents: 100}),
	}

	for name, html := range cases {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(html, payload) {
				t.Fatalf("raw brand payload leaked into %s email", name)
			}
			if !strings.Contains(html, escaped) {
				t.Fatalf("expected escaped brand in %s email", name)
			}
		})
	}
}
