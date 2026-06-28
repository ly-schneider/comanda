package service

import (
	"context"
	"strings"
	"testing"
)

func testBranding(name string) emailBranding {
	f := resolveFont("inter")
	r := resolveRadius("rounded")
	return emailBranding{
		Name:         name,
		FontName:     f.name,
		FontHref:     f.href,
		FontStack:    f.fontStack(),
		ButtonBg:     "#0a0a0a",
		ButtonText:   "#ffffff",
		IconRadius:   r.control,
		ButtonRadius: r.cta,
	}
}

func renderAll(t *testing.T, b emailBranding) map[string]string {
	t.Helper()
	ctx := context.Background()

	invite, err := renderInviteHTML(ctx, b, InviteEmailData{RoleLabel: "Admin", InviteURL: "https://x.test/i?a=1&b=2", ExpiresAt: "soon"})
	if err != nil {
		t.Fatalf("invite render: %v", err)
	}
	otp, err := renderOTPHTML(ctx, b, OTPEmailData{Code: "123456", CodeTTL: "5 Min", SupportNote: "Wir fragen nie nach deinem Code."}, OTPTypeSignIn)
	if err != nil {
		t.Fatalf("otp render: %v", err)
	}
	receipt, err := renderReceiptHTML(ctx, b, ReceiptEmailData{
		OrderID:    "ord_123",
		OrderURL:   "https://x.test/o",
		OrderDate:  "today",
		Method:     "card",
		TotalCents: 1250,
		Items: []ReceiptLineItem{
			{Title: "Pommes", Quantity: 2, Cents: 500, Children: []ReceiptLineItem{{Title: "Extra Sauce"}}},
		},
	})
	if err != nil {
		t.Fatalf("receipt render: %v", err)
	}
	return map[string]string{"invite": invite, "otp": otp, "receipt": receipt}
}

// A tenant-controlled display name (settings.display_name) flows into the
// branding, so it must never reach email HTML unescaped.
func TestEmailTemplatesEscapeBrand(t *testing.T) {
	const payload = `<img src=x onerror=alert(1)>`
	const escaped = "&lt;img src=x onerror=alert(1)&gt;"

	for name, htmlStr := range renderAll(t, testBranding(payload)) {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(htmlStr, payload) {
				t.Fatalf("raw brand payload leaked into %s email", name)
			}
			if !strings.Contains(htmlStr, escaped) {
				t.Fatalf("expected escaped brand in %s email", name)
			}
		})
	}
}

// Line item titles are also tenant-controlled (product names) and must be
// escaped where they're embedded into the <mj-table> rows.
func TestReceiptEscapesLineItems(t *testing.T) {
	html, err := renderReceiptHTML(context.Background(), testBranding("Acme"), ReceiptEmailData{
		OrderID:    "o1",
		OrderURL:   "https://x.test/o",
		OrderDate:  "today",
		Method:     "cash",
		TotalCents: 100,
		Items:      []ReceiptLineItem{{Title: `<b>x</b>`, Quantity: 1, Cents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<b>x</b>") {
		t.Fatal("raw line item title leaked into receipt email")
	}
	if !strings.Contains(html, "&lt;b&gt;x&lt;/b&gt;") {
		t.Fatal("expected escaped line item title")
	}
}

// Every email must carry the light-only lock and compile to real HTML.
func TestEmailTemplatesAreLightOnlyHTML(t *testing.T) {
	for name, htmlStr := range renderAll(t, testBranding("Acme")) {
		t.Run(name, func(t *testing.T) {
			if !strings.HasPrefix(strings.TrimSpace(htmlStr), "<!doctype html>") {
				t.Fatalf("%s email is not compiled HTML", name)
			}
			for _, want := range []string{
				`name="color-scheme" content="light"`,
				`name="supported-color-schemes" content="light"`,
				"data-ogsc",
				"fonts.googleapis.com",
			} {
				if !strings.Contains(htmlStr, want) {
					t.Fatalf("%s email missing %q", name, want)
				}
			}
		})
	}
}
