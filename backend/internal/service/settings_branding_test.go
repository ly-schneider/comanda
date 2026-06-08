package service

import (
	"context"
	"errors"
	"testing"

	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/settings"
	"backend/internal/repository"

	"github.com/google/uuid"
)

// fakeSettingsRepo records UpdateBranding calls and serves a canned Get result.
// Any access to settings on a platform-default path is a leak, so Get/TenantName
// flip a flag the platform test asserts stays false.
type fakeSettingsRepo struct {
	got      *ent.Settings
	tenant   string
	touched  bool
	branding *repository.BrandingUpdate
}

func (f *fakeSettingsRepo) Get(context.Context) (*ent.Settings, error) {
	f.touched = true
	if f.got != nil {
		return f.got, nil
	}
	return &ent.Settings{Currency: "CHF", ColorPrimary: "#f87778"}, nil
}
func (f *fakeSettingsRepo) GetWithProducts(context.Context) (*ent.Settings, error) {
	f.touched = true
	return f.Get(context.Background())
}
func (f *fakeSettingsRepo) Upsert(context.Context, settings.PosMode) error { return nil }
func (f *fakeSettingsRepo) UpdateClub100Settings(context.Context, []uuid.UUID, int) error {
	return nil
}
func (f *fakeSettingsRepo) IsSystemEnabled(context.Context) (bool, error) { return true, nil }
func (f *fakeSettingsRepo) SetSystemEnabled(context.Context, bool) error  { return nil }
func (f *fakeSettingsRepo) UpdateBranding(_ context.Context, b repository.BrandingUpdate) error {
	f.branding = &b
	return nil
}
func (f *fakeSettingsRepo) TenantName(context.Context) (string, error) {
	f.touched = true
	return f.tenant, nil
}
func (f *fakeSettingsRepo) EnsureWebhookToken(context.Context) (string, error) {
	return "test-token", nil
}
func (f *fakeSettingsRepo) UpsertPayrexxCredentials(context.Context, repository.PayrexxCredentials) error {
	return nil
}
func (f *fakeSettingsRepo) GetPayrexxCredentials(context.Context) (*repository.PayrexxCredentials, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeSettingsRepo) UpsertPlunkCredentials(context.Context, repository.PlunkCredentials) error {
	return nil
}
func (f *fakeSettingsRepo) GetPlunkCredentials(context.Context) (*repository.PlunkCredentials, error) {
	return nil, repository.ErrNotFound
}

func newSvc(repo repository.SettingsRepository) SettingsService {
	return &settingsService{settings: repo}
}

func strptr(s string) *string { return &s }

func TestUpdateBrandingValidation(t *testing.T) {
	tests := []struct {
		name    string
		in      BrandingInput
		wantErr bool
	}{
		{"valid 6-digit hex", BrandingInput{ColorPrimary: strptr("#A1B2C3")}, false},
		{"valid 8-digit hex (alpha)", BrandingInput{ColorPrimary: strptr("#A1B2C3FF")}, false},
		{"missing hash", BrandingInput{ColorPrimary: strptr("A1B2C3")}, true},
		{"too short", BrandingInput{ColorPrimary: strptr("#ABC")}, true},
		{"non-hex char", BrandingInput{ColorBackground: strptr("#GGGGGG")}, true},
		{"valid currency", BrandingInput{Currency: strptr("EUR")}, false},
		{"lowercase currency", BrandingInput{Currency: strptr("eur")}, true},
		{"4-letter currency", BrandingInput{Currency: strptr("EURO")}, true},
		{"nil colors ok", BrandingInput{DisplayName: strptr("Acme")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeSettingsRepo{}
			err := newSvc(repo).UpdateBranding(context.Background(), tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidBranding) {
					t.Fatalf("want ErrInvalidBranding, got %v", err)
				}
				if repo.branding != nil {
					t.Fatal("invalid input must not reach the repo")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.branding == nil {
				t.Fatal("valid input should reach the repo")
			}
		})
	}
}

func TestGetBrandingPlatformShortCircuit(t *testing.T) {
	repo := &fakeSettingsRepo{}
	ctx := enttenant.WithPlatform(context.Background())

	view, err := newSvc(repo).GetBranding(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.touched {
		t.Fatal("platform branding must not query the settings repo")
	}
	if view.DisplayName != "Comanda" {
		t.Fatalf("DisplayName = %q, want Comanda", view.DisplayName)
	}
	if view.Currency != "CHF" {
		t.Fatalf("Currency = %q, want CHF", view.Currency)
	}
	if view.ColorPrimary != "#f87778" {
		t.Fatalf("ColorPrimary = %q, want default", view.ColorPrimary)
	}
	if view.LogoURL != nil || view.FaviconURL != nil {
		t.Fatal("platform branding logo/favicon must be nil")
	}
}

func TestGetBrandingDisplayNameFallsBackToTenant(t *testing.T) {
	repo := &fakeSettingsRepo{
		got:    &ent.Settings{Currency: "CHF", ColorPrimary: "#111111"},
		tenant: "Acme Bistro",
	}
	// Tenant present (not platform) so the settings path runs.
	ctx := enttenant.WithTenant(context.Background(), uuid.New())

	view, err := newSvc(repo).GetBranding(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.DisplayName != "Acme Bistro" {
		t.Fatalf("DisplayName = %q, want tenant-name fallback", view.DisplayName)
	}
}

func TestGetBrandingPrefersSettingsDisplayName(t *testing.T) {
	repo := &fakeSettingsRepo{
		got:    &ent.Settings{Currency: "CHF", DisplayName: strptr("Custom Brand")},
		tenant: "Tenant Name",
	}
	ctx := enttenant.WithTenant(context.Background(), uuid.New())

	view, err := newSvc(repo).GetBranding(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.DisplayName != "Custom Brand" {
		t.Fatalf("DisplayName = %q, want settings value", view.DisplayName)
	}
}
