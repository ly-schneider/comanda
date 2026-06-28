package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"backend/internal/config"
	"backend/internal/crypto"
	"backend/internal/repository"

	"go.uber.org/zap"
)

const plunkSendEndpoint = "https://next-api.useplunk.com/v1/send"

type PlunkSendRequest struct {
	To         string            `json:"to"`
	Subject    string            `json:"subject"`
	Body       string            `json:"body"`
	Subscribed bool              `json:"subscribed"`
	Name       string            `json:"name,omitempty"`
	From       string            `json:"from,omitempty"`
	Reply      string            `json:"reply,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

type EmailService interface {
	SendInviteEmail(ctx context.Context, to string, inviteURL string, expiresAt time.Time) error
	SendOTPEmail(ctx context.Context, to string, otp string, otpType OTPType) error
	SendReceiptEmail(ctx context.Context, to string, data ReceiptEmailData) error
}

type emailService struct {
	cfg          config.PlunkConfig
	appEnv       string
	client       *http.Client
	settingsRepo repository.SettingsRepository
	encryptor    *crypto.Encryptor
	logger       *zap.Logger
}

func NewEmailService(cfg config.Config, settingsRepo repository.SettingsRepository, encryptor *crypto.Encryptor, logger *zap.Logger) EmailService {
	return &emailService{
		cfg:          cfg.Plunk,
		appEnv:       strings.ToLower(strings.TrimSpace(cfg.App.AppEnv)),
		client:       &http.Client{Timeout: 30 * time.Second},
		settingsRepo: settingsRepo,
		encryptor:    encryptor,
		logger:       logger,
	}
}

type plunkCreds struct {
	apiKey    string
	fromName  string
	fromEmail string
	replyTo   string
}

// resolveCreds falls back to the global config credentials only when no
// per-tenant credentials exist, keeping the bootstrap tenant and local dev
// working. ok=false means callers skip sending.
func (s *emailService) resolveCreds(ctx context.Context) (plunkCreds, bool) {
	brand := s.brandName(ctx)
	if s.encryptor != nil && s.settingsRepo != nil {
		creds, err := s.settingsRepo.GetPlunkCredentials(ctx)
		if err == nil {
			key, derr := s.encryptor.DecryptString(creds.APIKeyEnc)
			if derr != nil {
				s.logger.Error("plunk: failed to decrypt per-tenant api key", zap.Error(derr))
				return plunkCreds{}, false
			}
			return plunkCreds{
				apiKey:    key,
				fromName:  firstNonEmpty(creds.FromName, brand),
				fromEmail: firstNonEmpty(creds.FromEmail, s.cfg.FromEmail),
				replyTo:   firstNonEmpty(creds.ReplyTo, s.cfg.ReplyTo),
			}, true
		}
	}
	if s.cfg.APIKey != "" {
		return plunkCreds{
			apiKey:    s.cfg.APIKey,
			fromName:  firstNonEmpty(brand, s.cfg.FromName),
			fromEmail: s.cfg.FromEmail,
			replyTo:   s.cfg.ReplyTo,
		}, true
	}
	return plunkCreds{}, false
}

// brandName resolves the tenant's display name for the from-name and the email
// body: settings.display_name, else the tenant's name, else the platform
// default. On a platform/no-tenant context the RLS reads return nothing, so it
// falls back to "Comanda" — correct for platform mail like account-hub sign-in.
func (s *emailService) brandName(ctx context.Context) string {
	if s.settingsRepo != nil {
		if st, err := s.settingsRepo.Get(ctx); err == nil && st.DisplayName != nil {
			if name := strings.TrimSpace(*st.DisplayName); name != "" {
				return name
			}
		}
		if name, err := s.settingsRepo.TenantName(ctx); err == nil {
			if name = strings.TrimSpace(name); name != "" {
				return name
			}
		}
	}
	return firstNonEmpty(s.cfg.FromName, "Comanda")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *emailService) subject(base string) string {
	if s.appEnv == "staging" {
		return "[Staging] " + base
	}
	return base
}

func (s *emailService) SendInviteEmail(ctx context.Context, to string, inviteURL string, expiresAt time.Time) error {
	creds, ok := s.resolveCreds(ctx)
	if !ok {
		s.logger.Warn("Plunk not configured, skipping email",
			zap.String("to", to),
			zap.String("inviteURL", inviteURL),
		)
		return nil
	}

	data := InviteEmailData{
		Brand:     s.brandName(ctx),
		InviteURL: inviteURL,
		ExpiresAt: formatExpiry(expiresAt),
	}

	htmlBody := renderInviteHTML(data)

	payload := PlunkSendRequest{
		To:         to,
		Subject:    s.subject("Admin-Einladung"),
		Body:       htmlBody,
		Subscribed: false,
		Name:       creds.fromName,
		From:       creds.fromEmail,
		Reply:      creds.replyTo,
	}

	if err := s.send(ctx, creds, payload); err != nil {
		return err
	}

	s.logger.Info("sent invite email",
		zap.String("to", to),
	)

	return nil
}

func (s *emailService) send(ctx context.Context, creds plunkCreds, payload PlunkSendRequest) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, plunkSendEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+creds.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var respBody bytes.Buffer
		_, _ = respBody.ReadFrom(resp.Body)
		s.logger.Error("plunk error response",
			zap.Int("status", resp.StatusCode),
			zap.String("body", respBody.String()),
		)
		return fmt.Errorf("plunk returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *emailService) SendOTPEmail(ctx context.Context, to string, otp string, otpType OTPType) error {
	creds, ok := s.resolveCreds(ctx)
	if !ok {
		s.logger.Warn("Plunk not configured, skipping OTP email",
			zap.String("to", to),
			zap.String("type", string(otpType)),
		)
		return nil
	}

	// OTP expires in 5 minutes (300 seconds) - matches Better Auth config
	const otpExpiresInSeconds = 300

	data := OTPEmailData{
		Brand:       s.brandName(ctx),
		Code:        otp,
		CodeTTL:     friendlyTTL(otpExpiresInSeconds),
		SupportNote: "Wir werden dich niemals nach deinem Code fragen.",
	}

	htmlBody := renderOTPHTML(data)
	subject := s.subject(getOTPSubject(otpType))

	payload := PlunkSendRequest{
		To:         to,
		Subject:    subject,
		Body:       htmlBody,
		Subscribed: false,
		Name:       creds.fromName,
		From:       creds.fromEmail,
		Reply:      creds.replyTo,
	}

	if err := s.send(ctx, creds, payload); err != nil {
		return err
	}

	s.logger.Info("sent OTP email",
		zap.String("to", to),
		zap.String("type", string(otpType)),
	)

	return nil
}

func (s *emailService) SendReceiptEmail(ctx context.Context, to string, data ReceiptEmailData) error {
	creds, ok := s.resolveCreds(ctx)
	if !ok {
		s.logger.Warn("Plunk not configured, skipping receipt email",
			zap.String("to", to),
			zap.String("orderId", data.OrderID),
		)
		return nil
	}

	data.Brand = s.brandName(ctx)
	htmlBody := renderReceiptHTML(data)

	payload := PlunkSendRequest{
		To:         to,
		Subject:    s.subject("Deine Quittung von " + data.Brand),
		Body:       htmlBody,
		Subscribed: false,
		Name:       creds.fromName,
		From:       creds.fromEmail,
		Reply:      creds.replyTo,
	}

	if err := s.send(ctx, creds, payload); err != nil {
		return err
	}

	s.logger.Info("sent receipt email",
		zap.String("to", to),
		zap.String("orderId", data.OrderID),
	)

	return nil
}
