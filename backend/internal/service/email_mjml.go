package service

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	mjml "github.com/Boostport/mjml-go"
)

// Transactional emails are authored in MJML and compiled to bulletproof
// cross-client HTML by mjml-go (the real MJML compiler as WASM, in-process).
// A Go text/template emits MJML — loops/conditionals and the Swiss German copy
// stay in Go — and mjml-go handles the client-compatibility soup.
//
// The layout mirrors the app's auth screens: one flat background, content
// separated by padding (no floating card), an inline icon + name header, and a
// single CTA in the tenant's primary colour. Per-tenant: icon, name, font,
// roundness (radius_key) and the button colour. The neutral text palette
// (#0a0a0a / #737373 / #ffffff) is fixed so copy stays legible for any brand.
//
// Emails are light-only: pure #000 is avoided (#0a0a0a), Apple Mail is opted out
// of dark transforms via meta + color-scheme, and Outlook's forced inversion is
// re-asserted through the [data-ogsc]/[data-ogsb] selectors.

const (
	colBg      = "#ffffff"
	colHeading = "#0a0a0a"
	colBody    = "#737373"
	colBorder  = "#e5e5e5"
)

// emailBranding is the per-tenant slice of an email. Text colours are fixed; the
// icon, font, roundness and button colour come from the tenant. Name is raw
// (unescaped) — templates pipe it through esc.
type emailBranding struct {
	Name         string
	IconURL      string
	FontName     string
	FontHref     string
	FontStack    string
	ButtonBg     string
	ButtonText   string
	IconRadius   string
	ButtonRadius string
	SupportEmail string
	LegalName    string
	LegalAddress string
}

// fontSpec maps a tenant font_key to an email-usable font: a Google Fonts
// import (honoured by Apple/iOS Mail) plus a web-safe fallback stack for the
// clients (Gmail, Outlook) that strip custom fonts.
type fontSpec struct {
	name  string
	href  string
	stack string
}

const (
	sansFallback  = "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
	serifFallback = "Georgia, 'Times New Roman', serif"
)

func gFont(family string) string {
	return "https://fonts.googleapis.com/css2?family=" + family + ":wght@400;500;600;700&display=swap"
}

var fontByKey = map[string]fontSpec{
	"inter":          {"Inter", gFont("Inter"), sansFallback},
	"geist":          {"Geist", gFont("Geist"), sansFallback},
	"space-grotesk":  {"Space Grotesk", gFont("Space+Grotesk"), sansFallback},
	"ibm-plex-sans":  {"IBM Plex Sans", gFont("IBM+Plex+Sans"), sansFallback},
	"jetbrains-mono": {"JetBrains Mono", gFont("JetBrains+Mono"), sansFallback},
	"lora":           {"Lora", gFont("Lora"), serifFallback},
}

func resolveFont(fontKey string) fontSpec {
	if f, ok := fontByKey[fontKey]; ok {
		return f
	}
	return fontByKey["inter"]
}

// fontStack is the full CSS font-family applied to text: the tenant font first,
// then the fallback stack.
func (f fontSpec) fontStack() string {
	return "'" + f.name + "', " + f.stack
}

// radiusSpec mirrors the web RADIUS_KEY_TO_VALUES so emails round like the app.
type radiusSpec struct{ control, cta string }

var radiusByKey = map[string]radiusSpec{
	"none":    {"0px", "0px"},
	"rounded": {"10px", "9999px"},
	"pill":    {"9999px", "9999px"},
}

func resolveRadius(radiusKey string) radiusSpec {
	if r, ok := radiusByKey[radiusKey]; ok {
		return r
	}
	return radiusByKey["rounded"]
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)

// safeHex guards tenant colours before they're interpolated into MJML/CSS
// (defense-in-depth — colours are already hex-validated on write).
func safeHex(v, fallback string) string {
	if hexRe.MatchString(strings.TrimSpace(v)) {
		return strings.TrimSpace(v)
	}
	return fallback
}

type baseView struct {
	Subject  string
	Preview  string
	Branding emailBranding
}

func newBaseView(subject, preview string, b emailBranding) baseView {
	return baseView{Subject: subject, Preview: preview, Branding: b}
}

var emailFuncs = template.FuncMap{"esc": html.EscapeString}

var (
	otpTmpl     = template.Must(template.New("email").Funcs(emailFuncs).Parse(baseMJML + otpContentMJML))
	inviteTmpl  = template.Must(template.New("email").Funcs(emailFuncs).Parse(baseMJML + inviteContentMJML))
	receiptTmpl = template.Must(template.New("email").Funcs(emailFuncs).Parse(baseMJML + receiptContentMJML))
)

func renderMJML(ctx context.Context, tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", fmt.Errorf("render mjml template: %w", err)
	}
	out, err := mjml.ToHTML(ctx, buf.String(), mjml.WithMinify(true), mjml.WithValidationLevel(mjml.Soft))
	if err != nil {
		return "", fmt.Errorf("compile mjml: %w", err)
	}
	return out, nil
}

var warmOnce sync.Once

// warmMJML pays the one-time WASM/compiler init cost off the request path so the
// first real email isn't slow.
func warmMJML() {
	warmOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = mjml.ToHTML(ctx, "<mjml><mj-body></mj-body></mjml>")
	})
}

const baseMJML = `{{define "base"}}<mjml>
  <mj-head>
    <mj-title>{{.Subject | esc}}</mj-title>
    <mj-preview>{{.Preview | esc}}</mj-preview>
    <mj-attributes>
      <mj-all font-family="{{.Branding.FontStack}}" />
      <mj-text font-size="15px" line-height="1.6" color="` + colBody + `" padding="0" />
      <mj-button padding="0" />
      <mj-table padding="0" />
      <mj-image padding="0" />
      <mj-divider padding="0" />
    </mj-attributes>
    {{if .Branding.FontHref}}<mj-font name="{{.Branding.FontName}}" href="{{.Branding.FontHref}}" />{{end}}
    <mj-style>
      :root { color-scheme: light only; supported-color-schemes: light; }
      a { color: ` + colHeading + `; }
      [data-ogsb] .page { background: ` + colBg + ` !important; }
      [data-ogsc] .heading { color: ` + colHeading + ` !important; }
      [data-ogsc] .body { color: ` + colBody + ` !important; }
      [data-ogsb] .btn td { background: {{.Branding.ButtonBg}} !important; }
      [data-ogsc] .btn a { color: {{.Branding.ButtonText}} !important; }
    </mj-style>
    <mj-raw>
      <meta name="color-scheme" content="light">
      <meta name="supported-color-schemes" content="light">
    </mj-raw>
  </mj-head>
  <mj-body background-color="` + colBg + `" css-class="page" width="600px">
    <mj-section padding="44px 44px 30px 44px">
      <mj-column>
        <mj-text>{{if .Branding.IconURL}}<img src="{{.Branding.IconURL | esc}}" alt="{{.Branding.Name | esc}}" height="40" style="height:40px;width:auto;max-width:150px;border-radius:{{.Branding.IconRadius}};vertical-align:middle;display:inline-block;" /> {{end}}<span style="vertical-align:middle;font-size:17px;font-weight:700;color:` + colHeading + `;{{if .Branding.IconURL}}padding-left:12px;{{end}}">{{.Branding.Name | esc}}</span></mj-text>
      </mj-column>
    </mj-section>
    {{template "content" .}}
    <mj-section padding="34px 44px 44px 44px">
      <mj-column>
        <mj-text css-class="body" font-size="12px" color="` + colBody + `" line-height="1.7">
          <strong class="heading" style="color:` + colHeading + `;">{{.Branding.Name | esc}}</strong><br/>
          {{if .Branding.LegalName}}{{.Branding.LegalName | esc}}<br/>{{end}}
          {{if .Branding.LegalAddress}}{{.Branding.LegalAddress | esc}}<br/>{{end}}
          {{if .Branding.SupportEmail}}<a class="body" style="color:` + colBody + `;" href="mailto:{{.Branding.SupportEmail | esc}}">{{.Branding.SupportEmail | esc}}</a> &middot; {{end}}Powered by Leys Services
        </mj-text>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>{{end}}`
