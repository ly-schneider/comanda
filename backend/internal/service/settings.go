package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/settings"
	"backend/internal/repository"
)

const systemEnabledCacheTTL = 5 * time.Second

type systemEnabledCache struct {
	enabled   bool
	expiresAt time.Time
}

var (
	ErrJetonRequired = errors.New("jeton_required")
	// ErrInvalidBranding signals a 400 to the handler (invalid hex / currency).
	ErrInvalidBranding = errors.New("invalid_branding")
)

var (
	hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// allowedFontKeys / allowedRadiusKeys gate the curated branding choices. The web
// maps each key to a constant (a next/font variable, a radius value); the raw
// string never reaches the CSS-var injection, so the allow-list is the security
// boundary. Keep in sync with web/lib/fonts.ts and the corner-style picker.
var (
	allowedFontKeys = map[string]struct{}{
		"inter": {}, "geist": {}, "space-grotesk": {},
		"ibm-plex-sans": {}, "jetbrains-mono": {}, "lora": {},
	}
	allowedRadiusKeys = map[string]struct{}{
		"none": {}, "rounded": {}, "pill": {},
	}
)

// BrandingView is the resolved public branding for a tenant. DisplayName is
// already resolved (settings.display_name, else tenant name). Nil string fields
// map to JSON null.
type BrandingView struct {
	DisplayName            string
	LogoURL                *string
	FaviconURL             *string
	ColorPrimary           string
	ColorPrimaryForeground string
	ColorSecondary         string
	ColorBackground        string
	ColorForeground        string
	FontKey                string
	RadiusKey              string
	ContactEmail           *string
	ContactPhone           *string
	LegalName              *string
	LegalAddress           *string
	Currency               string
}

// PlatformBrandingDefaults is the neutral branding returned on a platform host
// (no tenant). It never touches the database; it mirrors the fixed white base.
func PlatformBrandingDefaults() *BrandingView {
	return &BrandingView{
		DisplayName:            "Comanda",
		ColorPrimary:           "#0a0a0a",
		ColorPrimaryForeground: "#ffffff",
		ColorSecondary:         "#f5f5f5",
		ColorBackground:        "#ffffff",
		ColorForeground:        "#0a0a0a",
		FontKey:                "inter",
		RadiusKey:              "rounded",
		Currency:               "CHF",
	}
}

// BrandingInput is the validated branding patch from the admin API. Nil pointer
// = leave unchanged; pointer to "" on a nillable text field = clear (NULL).
// Colors and currency, when present, must be non-empty and well-formed.
type BrandingInput struct {
	DisplayName       *string
	LogoURL           *string
	FaviconURL        *string
	ColorPrimary      *string
	ColorPrimaryFg    *string
	ColorSecondary    *string
	ColorBackground   *string
	ColorForeground   *string
	FontKey           *string
	RadiusKey         *string
	ContactEmail      *string
	ContactPhone      *string
	LegalName         *string
	LegalAddress      *string
	Currency          *string
	PaymentProvider   *string
	SumupMerchantCode *string
}

type MissingJetonForActiveProductsError struct {
	Count int64
}

func (e MissingJetonForActiveProductsError) Error() string {
	return "active_products_missing_jeton"
}

type JetonInUseError struct {
	Count int64
}

func (e JetonInUseError) Error() string {
	return "jeton_in_use"
}

type SettingsService interface {
	GetSettings(ctx context.Context) (*ent.Settings, error)
	GetSettingsWithProducts(ctx context.Context) (*ent.Settings, error)
	SetPosMode(ctx context.Context, mode settings.PosMode) error
	IsSystemEnabled(ctx context.Context) (bool, error)
	SetSystemEnabled(ctx context.Context, enabled bool) error
	SetOnboardingCompleted(ctx context.Context, completed bool) error
	UpdateBranding(ctx context.Context, in BrandingInput) error
	GetBranding(ctx context.Context) (*BrandingView, error)
	ListJetons(ctx context.Context) ([]*ent.Jeton, error)
	CreateJeton(ctx context.Context, name, color string) (*ent.Jeton, error)
	UpdateJeton(ctx context.Context, id string, name, color string) (*ent.Jeton, error)
	DeleteJeton(ctx context.Context, id string) error
	SetProductJeton(ctx context.Context, productID string, jetonID *string) error
}

type settingsService struct {
	settings           repository.SettingsRepository
	jetons             repository.JetonRepository
	products           *repository.ProductRepository
	systemEnabledCache atomic.Pointer[systemEnabledCache]
}

func NewSettingsService(
	settings repository.SettingsRepository,
	jetons repository.JetonRepository,
	products *repository.ProductRepository,
) SettingsService {
	return &settingsService{settings: settings, jetons: jetons, products: products}
}

var settingsHexPattern = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$`)

func normalizeSettingsColor(raw string) (string, error) {
	h := strings.TrimSpace(raw)
	if h == "" {
		return "", fmt.Errorf("color_required")
	}
	if !settingsHexPattern.MatchString(h) {
		return "", fmt.Errorf("invalid_hex")
	}
	h = strings.ToUpper(h)
	if !strings.HasPrefix(h, "#") {
		h = "#" + h
	}
	return h, nil
}

func (s *settingsService) GetSettings(ctx context.Context) (*ent.Settings, error) {
	return s.settings.Get(ctx)
}

func (s *settingsService) GetSettingsWithProducts(ctx context.Context) (*ent.Settings, error) {
	return s.settings.GetWithProducts(ctx)
}

func (s *settingsService) SetPosMode(ctx context.Context, mode settings.PosMode) error {
	if mode != settings.PosModeQR_CODE && mode != settings.PosModeJETON && mode != settings.PosModeHYBRID {
		return fmt.Errorf("invalid_mode")
	}
	if mode == settings.PosModeJETON || mode == settings.PosModeHYBRID {
		if missing, err := s.products.CountActiveWithoutJeton(ctx); err == nil && missing > 0 {
			return MissingJetonForActiveProductsError{Count: missing}
		} else if err != nil {
			return err
		}
	}
	return s.settings.Upsert(ctx, mode)
}

func (s *settingsService) IsSystemEnabled(ctx context.Context) (bool, error) {
	if cached := s.systemEnabledCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.enabled, nil
	}
	enabled, err := s.settings.IsSystemEnabled(ctx)
	if err != nil {
		return enabled, err
	}
	s.systemEnabledCache.Store(&systemEnabledCache{
		enabled:   enabled,
		expiresAt: time.Now().Add(systemEnabledCacheTTL),
	})
	return enabled, nil
}

func (s *settingsService) SetSystemEnabled(ctx context.Context, enabled bool) error {
	if err := s.settings.SetSystemEnabled(ctx, enabled); err != nil {
		return err
	}
	s.systemEnabledCache.Store(&systemEnabledCache{
		enabled:   enabled,
		expiresAt: time.Now().Add(systemEnabledCacheTTL),
	})
	return nil
}

func (s *settingsService) SetOnboardingCompleted(ctx context.Context, completed bool) error {
	return s.settings.SetOnboardingCompleted(ctx, completed)
}

func validateBrandingColor(v *string) error {
	if v == nil {
		return nil
	}
	if !hexColorPattern.MatchString(*v) {
		return ErrInvalidBranding
	}
	return nil
}

func (s *settingsService) UpdateBranding(ctx context.Context, in BrandingInput) error {
	for _, c := range []*string{in.ColorPrimary, in.ColorPrimaryFg, in.ColorSecondary, in.ColorBackground, in.ColorForeground} {
		if err := validateBrandingColor(c); err != nil {
			return err
		}
	}
	if in.Currency != nil && !currencyPattern.MatchString(*in.Currency) {
		return ErrInvalidBranding
	}
	if in.FontKey != nil {
		if _, ok := allowedFontKeys[*in.FontKey]; !ok {
			return ErrInvalidBranding
		}
	}
	if in.RadiusKey != nil {
		if _, ok := allowedRadiusKeys[*in.RadiusKey]; !ok {
			return ErrInvalidBranding
		}
	}
	if in.SumupMerchantCode != nil {
		trimmed := strings.TrimSpace(*in.SumupMerchantCode)
		in.SumupMerchantCode = &trimmed
	}

	return s.settings.UpdateBranding(ctx, repository.BrandingUpdate{
		DisplayName:            in.DisplayName,
		LogoURL:                in.LogoURL,
		FaviconURL:             in.FaviconURL,
		ColorPrimary:           in.ColorPrimary,
		ColorPrimaryForeground: in.ColorPrimaryFg,
		ColorSecondary:         in.ColorSecondary,
		ColorBackground:        in.ColorBackground,
		ColorForeground:        in.ColorForeground,
		FontKey:                in.FontKey,
		RadiusKey:              in.RadiusKey,
		ContactEmail:           in.ContactEmail,
		ContactPhone:           in.ContactPhone,
		LegalName:              in.LegalName,
		LegalAddress:           in.LegalAddress,
		Currency:               in.Currency,
		PaymentProvider:        in.PaymentProvider,
		SumupMerchantCode:      in.SumupMerchantCode,
	})
}

func (s *settingsService) GetBranding(ctx context.Context) (*BrandingView, error) {
	// Platform (no-tenant) host: return neutral defaults without touching the
	// DB. The interceptor fail-closes on tenant-scoped reads with no tenant.
	if enttenant.IsPlatform(ctx) {
		return PlatformBrandingDefaults(), nil
	}

	e, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}

	displayName := ""
	if e.DisplayName != nil {
		displayName = strings.TrimSpace(*e.DisplayName)
	}
	if displayName == "" {
		if name, err := s.settings.TenantName(ctx); err == nil {
			displayName = name
		}
	}

	return &BrandingView{
		DisplayName:            displayName,
		LogoURL:                e.LogoURL,
		FaviconURL:             e.FaviconURL,
		ColorPrimary:           e.ColorPrimary,
		ColorPrimaryForeground: e.ColorPrimaryForeground,
		ColorSecondary:         e.ColorSecondary,
		ColorBackground:        e.ColorBackground,
		ColorForeground:        e.ColorForeground,
		FontKey:                e.FontKey,
		RadiusKey:              e.RadiusKey,
		ContactEmail:           e.ContactEmail,
		ContactPhone:           e.ContactPhone,
		LegalName:              e.LegalName,
		LegalAddress:           e.LegalAddress,
		Currency:               e.Currency,
	}, nil
}

func (s *settingsService) ListJetons(ctx context.Context) ([]*ent.Jeton, error) {
	return s.jetons.GetAll(ctx)
}

func (s *settingsService) CreateJeton(ctx context.Context, name, color string) (*ent.Jeton, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name_required")
	}
	normColor, err := normalizeSettingsColor(color)
	if err != nil {
		return nil, err
	}
	return s.jetons.Create(ctx, name, normColor)
}

func (s *settingsService) UpdateJeton(ctx context.Context, id string, name, color string) (*ent.Jeton, error) {
	if _, err := s.jetons.GetByID(ctx, id); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name_required")
	}
	normColor, err := normalizeSettingsColor(color)
	if err != nil {
		return nil, err
	}
	return s.jetons.Update(ctx, id, name, normColor)
}

func (s *settingsService) DeleteJeton(ctx context.Context, id string) error {
	usage, err := s.products.CountByJetonIDs(ctx, []string{id})
	if err == nil {
		if c := usage[id]; c > 0 {
			return JetonInUseError{Count: c}
		}
	}
	return s.jetons.Delete(ctx, id)
}

func (s *settingsService) SetProductJeton(ctx context.Context, productID string, jetonID *string) error {
	p, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return err
	}
	if jetonID != nil {
		if _, err := s.jetons.GetByID(ctx, *jetonID); err != nil {
			return err
		}
	}
	settingsData, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	if settingsData != nil && (settingsData.PosMode == settings.PosModeJETON || settingsData.PosMode == settings.PosModeHYBRID) && p.IsActive && jetonID == nil {
		if p.Type == "simple" {
			return ErrJetonRequired
		}
	}
	return s.products.UpdateJeton(ctx, productID, jetonID)
}
