package service

import (
	"context"
	"errors"
	"testing"

	"backend/internal/config"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenant"
)

type fakeTenantRepo struct {
	created        *ent.Tenant
	seededSettings string
	seededDomain   string
	createErr      error
}

func (f *fakeTenantRepo) Create(_ context.Context, slug, name string) (*ent.Tenant, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = &ent.Tenant{ID: "tnt_test____", Slug: slug, Name: name, Status: tenant.StatusActive}
	return f.created, nil
}
func (f *fakeTenantRepo) List(context.Context) ([]*ent.Tenant, error) { return nil, nil }
func (f *fakeTenantRepo) GetByID(context.Context, string) (*ent.Tenant, error) {
	return f.created, nil
}
func (f *fakeTenantRepo) SetStatus(_ context.Context, _ string, status tenant.Status) (*ent.Tenant, error) {
	if f.created == nil {
		f.created = &ent.Tenant{ID: "tnt_test____"}
	}
	f.created.Status = status
	return f.created, nil
}
func (f *fakeTenantRepo) SeedSettings(_ context.Context, tenantID string) error {
	f.seededSettings = tenantID
	return nil
}
func (f *fakeTenantRepo) SeedPrimaryDomain(_ context.Context, _, hostname string) error {
	f.seededDomain = hostname
	return nil
}

func newTenantSvc(repo *fakeTenantRepo, base string) TenantService {
	return NewTenantService(repo, config.Config{CustomDomain: config.CustomDomainConfig{BaseDomain: base}})
}

func TestCreateTenantUsesFlatPlatformDomain(t *testing.T) {
	repo := &fakeTenantRepo{}
	cfg := config.Config{CustomDomain: config.CustomDomainConfig{BaseDomain: "leys.ch", TenantLabelSuffix: "-comanda-staging"}}
	_, err := NewTenantService(repo, cfg).Create(context.Background(), "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	if repo.seededDomain != "acme-comanda-staging.leys.ch" {
		t.Fatalf("unexpected primary domain: %s", repo.seededDomain)
	}
}

func TestCreateTenantRejectsOverlongFlatLabel(t *testing.T) {
	repo := &fakeTenantRepo{}
	cfg := config.Config{CustomDomain: config.CustomDomainConfig{BaseDomain: "leys.ch", TenantLabelSuffix: "-comanda-staging"}}
	_, err := NewTenantService(repo, cfg).Create(context.Background(), "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz", "Acme")
	if !errors.Is(err, ErrInvalidSlug) || repo.created != nil {
		t.Fatal("an overlong DNS label must fail before creating a tenant")
	}
}

func TestCreateTenantValidation(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		tenName string
		wantErr error
	}{
		{"valid", "acme", "Acme Bistro", nil},
		{"uppercase slug normalized", "ACME", "Acme", nil},
		{"empty name", "acme", "  ", ErrInvalidName},
		{"slug with space", "ac me", "Acme", ErrInvalidSlug},
		{"slug leading hyphen", "-acme", "Acme", ErrInvalidSlug},
		{"reserved slug admin", "admin", "Acme", ErrReservedSlug},
		{"reserved slug platform", "platform", "Acme", ErrReservedSlug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeTenantRepo{}
			_, err := newTenantSvc(repo, "comanda.leys.ch").Create(context.Background(), tt.slug, tt.tenName)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want %v, got %v", tt.wantErr, err)
				}
				if repo.created != nil {
					t.Fatal("invalid input must not create a tenant")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.seededSettings != repo.created.ID {
				t.Fatalf("settings must be seeded for the new tenant id")
			}
		})
	}
}

func TestCreateTenantSeedsPrimaryDomain(t *testing.T) {
	repo := &fakeTenantRepo{}
	_, err := newTenantSvc(repo, "comanda.leys.ch").Create(context.Background(), "acme", "Acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.seededDomain != "acme.comanda.leys.ch" {
		t.Fatalf("primary domain = %q, want acme.comanda.leys.ch", repo.seededDomain)
	}
}

func TestCreateTenantNoBaseDomainSkipsDomain(t *testing.T) {
	repo := &fakeTenantRepo{}
	_, err := newTenantSvc(repo, "").Create(context.Background(), "acme", "Acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.seededDomain != "" {
		t.Fatalf("no base domain configured: must not seed a domain, got %q", repo.seededDomain)
	}
}

func TestSetTenantStatusValidation(t *testing.T) {
	repo := &fakeTenantRepo{created: &ent.Tenant{ID: "tnt_test____"}}
	svc := newTenantSvc(repo, "")

	if _, err := svc.SetStatus(context.Background(), "tnt_test____", "suspended"); err != nil {
		t.Fatalf("suspended should be valid: %v", err)
	}
	if repo.created.Status != tenant.StatusSuspended {
		t.Fatalf("status = %q, want suspended", repo.created.Status)
	}
	if _, err := svc.SetStatus(context.Background(), "tnt_test____", "bogus"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("want ErrInvalidStatus, got %v", err)
	}
}
