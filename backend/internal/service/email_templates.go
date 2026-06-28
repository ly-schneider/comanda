package service

import (
	"context"
	"fmt"
	"time"
)

// InviteEmailData contains the data for rendering an admin invite email.
type InviteEmailData struct {
	RoleLabel string
	InviteURL string
	ExpiresAt string
}

// formatExpiry formats a time.Time in German Swiss locale.
func formatExpiry(t time.Time) string {
	// Format as "2. Januar 2025, 14:30"
	loc, _ := time.LoadLocation("Europe/Zurich")
	t = t.In(loc)

	months := []string{
		"Januar", "Februar", "März", "April", "Mai", "Juni",
		"Juli", "August", "September", "Oktober", "November", "Dezember",
	}
	month := months[t.Month()-1]

	return fmt.Sprintf("%d. %s %d, %02d:%02d", t.Day(), month, t.Year(), t.Hour(), t.Minute())
}

type inviteView struct {
	baseView
	RoleLabel string
	URL       string
	ExpiresAt string
}

// renderInviteHTML renders the admin invite email to HTML.
func renderInviteHTML(ctx context.Context, b emailBranding, data InviteEmailData) (string, error) {
	view := inviteView{
		baseView:  newBaseView("Einladung als "+data.RoleLabel, "Du wurdest eingeladen, als "+data.RoleLabel+" bei "+b.Name+" beizutreten.", b),
		RoleLabel: data.RoleLabel,
		URL:       data.InviteURL,
		ExpiresAt: data.ExpiresAt,
	}
	return renderMJML(ctx, inviteTmpl, view)
}

const inviteContentMJML = `{{define "content"}}
<mj-section padding="26px 24px 4px 24px">
  <mj-column>
    <mj-text css-class="heading" font-size="21px" font-weight="700" color="#1A1A1A" line-height="1.3" padding="0 0 8px 0">Einladung als {{.RoleLabel | esc}}</mj-text>
    <mj-text css-class="body" color="#6E6E6E" line-height="1.6">Du wurdest eingeladen, als {{.RoleLabel | esc}} bei {{.Branding.Name | esc}} beizutreten. Klicke auf den Button unten, um die Einladung anzunehmen.</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="18px 24px 6px 24px">
  <mj-column>
    <mj-button css-class="btn" width="100%" background-color="#1A1A1A" color="#FAFAFA" border-radius="10px" font-size="15px" font-weight="600" inner-padding="15px 24px" href="{{.URL | esc}}">Einladung annehmen</mj-button>
  </mj-column>
</mj-section>
<mj-section padding="6px 24px 0 24px">
  <mj-column>
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.6">Diese Einladung ist gültig bis: {{.ExpiresAt | esc}}</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="14px 24px 26px 24px">
  <mj-column>
    <mj-divider border-width="1px" border-color="#E6E4E2" padding="0 0 14px 0" />
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.6" padding="0 0 6px 0">Falls der Button nicht funktioniert, kopiere diesen Link in deinen Browser:</mj-text>
    <mj-text css-class="muted" font-family="{{.MonoStack}}" font-size="11px" color="#9A9A9A" line-height="1.5">{{.URL | esc}}</mj-text>
  </mj-column>
</mj-section>
{{end}}`

// OTPType represents the type of OTP being sent.
type OTPType string

const (
	OTPTypeSignIn            OTPType = "sign-in"
	OTPTypeEmailVerification OTPType = "email-verification"
	OTPTypeForgetPassword    OTPType = "forget-password"
)

// OTPEmailData contains the data for rendering an OTP email.
type OTPEmailData struct {
	Code        string
	CodeTTL     string
	SupportNote string
}

// friendlyTTL formats duration in a human-friendly German format.
func friendlyTTL(seconds int) string {
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24

	var parts []string

	if days > 0 {
		if days == 1 {
			parts = append(parts, "1 Tag")
		} else {
			parts = append(parts, fmt.Sprintf("%d Tagen", days))
		}
	}

	remainingHours := hours % 24
	if remainingHours > 0 {
		parts = append(parts, fmt.Sprintf("%d Std", remainingHours))
	}

	remainingMinutes := minutes % 60
	if remainingMinutes > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d Min", remainingMinutes))
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

// getOTPSubject returns the email subject based on OTP type.
func getOTPSubject(otpType OTPType) string {
	switch otpType {
	case OTPTypeSignIn:
		return "Dein Anmeldecode"
	case OTPTypeForgetPassword:
		return "Passwort zurücksetzen"
	default:
		return "Dein Verifizierungscode"
	}
}

// otpHeading/otpIntro adapt the copy to the OTP flow; the code panel and
// reassurance are shared.
func otpHeading(otpType OTPType) string {
	switch otpType {
	case OTPTypeSignIn:
		return "Anmelden mit Code"
	case OTPTypeForgetPassword:
		return "Passwort zurücksetzen"
	default:
		return "E-Mail bestätigen"
	}
}

func otpIntro(otpType OTPType) string {
	switch otpType {
	case OTPTypeSignIn:
		return "Gib diesen einmaligen Code im Anmeldefenster ein:"
	case OTPTypeForgetPassword:
		return "Gib diesen Code ein, um ein neues Passwort zu setzen:"
	default:
		return "Gib diesen Code ein, um deine E-Mail-Adresse zu bestätigen:"
	}
}

type otpView struct {
	baseView
	Heading     string
	Intro       string
	Code        string
	TTLNote     string
	SupportNote string
}

// renderOTPHTML renders the one-time-code email to HTML.
func renderOTPHTML(ctx context.Context, b emailBranding, data OTPEmailData, otpType OTPType) (string, error) {
	heading := otpHeading(otpType)
	view := otpView{
		baseView:    newBaseView(heading, heading+" bei "+b.Name, b),
		Heading:     heading,
		Intro:       otpIntro(otpType),
		Code:        data.Code,
		TTLNote:     "Der Code läuft in " + data.CodeTTL + " ab und kann nur einmal verwendet werden.",
		SupportNote: data.SupportNote,
	}
	return renderMJML(ctx, otpTmpl, view)
}

const otpContentMJML = `{{define "content"}}
<mj-section padding="26px 24px 4px 24px">
  <mj-column>
    <mj-text css-class="heading" font-size="21px" font-weight="700" color="#1A1A1A" line-height="1.3" padding="0 0 8px 0">{{.Heading | esc}}</mj-text>
    <mj-text css-class="body" color="#6E6E6E" line-height="1.6">{{.Intro | esc}}</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="12px 24px">
  <mj-column css-class="panel" background-color="#F4F3F1" border="1px solid #E6E4E2" border-radius="10px" padding="22px 12px">
    <mj-text css-class="code" align="center" font-family="{{.MonoStack}}" font-size="32px" font-weight="700" letter-spacing="8px" color="#1A1A1A" line-height="1.1">{{.Code | esc}}</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="10px 24px 0 24px">
  <mj-column>
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.6">{{.TTLNote | esc}}</mj-text>
  </mj-column>
</mj-section>
<mj-section padding="14px 24px 26px 24px">
  <mj-column>
    <mj-divider border-width="1px" border-color="#E6E4E2" padding="0 0 14px 0" />
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.6" padding="0 0 4px 0">{{.SupportNote | esc}}</mj-text>
    <mj-text css-class="muted" font-size="12px" color="#9A9A9A" line-height="1.6">Wenn du dies nicht angefordert hast, kannst du diese E-Mail ignorieren.</mj-text>
  </mj-column>
</mj-section>
{{end}}`
