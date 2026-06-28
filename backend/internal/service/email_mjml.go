package service

import (
	"bytes"
	"context"
	"fmt"
	"html"
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
// Design is a fixed, tenant-agnostic neutral palette (Slack-style): only the
// logo, display name and font are per-tenant. Colours never come from the
// tenant brand so every tenant's mail stays legible. Emails are light-only:
// pure #000/#fff are avoided (Gmail leaves slightly-off values alone), Apple
// Mail is opted out of dark transforms via meta + color-scheme, and Outlook's
// forced inversion is re-asserted through the [data-ogsc]/[data-ogsb] selectors.

const (
	colPage      = "#F2F1EF"
	colCard      = "#FDFDFD"
	colBorder    = "#E6E4E2"
	colHeading   = "#1A1A1A"
	colBody      = "#6E6E6E"
	colMuted     = "#9A9A9A"
	colPanel     = "#F4F3F1"
	colButtonBg  = "#1A1A1A"
	colButtonTxt = "#FAFAFA"

	monoStack = "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace"
)

// emailBranding is the per-tenant slice of an email: name, logo and font come
// from the tenant; the rest of the design is fixed. Name is raw (unescaped) —
// templates pipe it through esc.
type emailBranding struct {
	Name         string
	LogoURL      string
	FontName     string
	FontHref     string
	FontStack    string
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
	"jetbrains-mono": {"JetBrains Mono", gFont("JetBrains+Mono"), monoStack},
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

type baseView struct {
	Subject   string
	Preview   string
	Branding  emailBranding
	MonoStack string
}

func newBaseView(subject, preview string, b emailBranding) baseView {
	return baseView{Subject: subject, Preview: preview, Branding: b, MonoStack: monoStack}
}

var emailFuncs = template.FuncMap{"esc": html.EscapeString}

var (
	otpTmpl     = template.Must(template.New("email").Funcs(emailFuncs).Parse(baseMJML + otpContentMJML))
	inviteTmpl  = template.Must(template.New("email").Funcs(emailFuncs).Parse(baseMJML + inviteContentMJML))
	receiptTmpl = template.Must(template.New("email").Funcs(emailFuncs).Parse(baseMJML + receiptContentMJML))
)

// renderMJML executes an email template to MJML, then compiles it to HTML.
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
      <mj-text font-size="14px" line-height="1.6" color="` + colBody + `" padding="0" />
    </mj-attributes>
    {{if .Branding.FontHref}}<mj-font name="{{.Branding.FontName}}" href="{{.Branding.FontHref}}" />{{end}}
    <mj-style>
      :root { color-scheme: light only; supported-color-schemes: light; }
      a { color: ` + colHeading + `; }
      [data-ogsb] .page { background: ` + colPage + ` !important; }
      [data-ogsb] .card { background: ` + colCard + ` !important; }
      [data-ogsb] .panel { background: ` + colPanel + ` !important; }
      [data-ogsb] .btn td { background: ` + colButtonBg + ` !important; }
      [data-ogsc] .heading { color: ` + colHeading + ` !important; }
      [data-ogsc] .body { color: ` + colBody + ` !important; }
      [data-ogsc] .muted { color: ` + colMuted + ` !important; }
      [data-ogsc] .code { color: ` + colHeading + ` !important; }
      [data-ogsc] .btn a { color: ` + colButtonTxt + ` !important; }
    </mj-style>
    <mj-raw>
      <meta name="color-scheme" content="light">
      <meta name="supported-color-schemes" content="light">
    </mj-raw>
  </mj-head>
  <mj-body background-color="` + colPage + `" css-class="page" width="600px">
    <mj-section padding="32px 24px 14px 24px">
      <mj-column>
        {{if .Branding.LogoURL}}<mj-image src="{{.Branding.LogoURL | esc}}" alt="{{.Branding.Name | esc}}" align="left" width="130px" height="auto" padding="0 0 10px 0" />{{end}}
        <mj-text css-class="heading" font-size="15px" font-weight="700" color="` + colHeading + `" line-height="1.2">{{.Branding.Name | esc}}</mj-text>
      </mj-column>
    </mj-section>
    <mj-wrapper css-class="card" background-color="` + colCard + `" border="1px solid ` + colBorder + `" border-radius="14px" padding="0">
      {{template "content" .}}
    </mj-wrapper>
    <mj-section padding="18px 24px 36px 24px">
      <mj-column>
        <mj-text css-class="muted" font-size="12px" color="` + colMuted + `" line-height="1.7">
          <strong class="heading" style="color:` + colHeading + `;">{{.Branding.Name | esc}}</strong><br/>
          {{if .Branding.LegalAddress}}{{.Branding.LegalAddress | esc}}<br/>{{end}}
          {{if .Branding.SupportEmail}}<a class="muted" style="color:` + colMuted + `;" href="mailto:{{.Branding.SupportEmail | esc}}">{{.Branding.SupportEmail | esc}}</a> &middot; {{end}}Powered by Leys
        </mj-text>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>{{end}}`
