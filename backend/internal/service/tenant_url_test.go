package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/generated/ent"
)

type fakeDomainLister struct {
	rows []*ent.TenantDomain
	err  error
}

func (f *fakeDomainLister) List(context.Context) ([]*ent.TenantDomain, error) {
	return f.rows, f.err
}

func verified(host string, primary bool) *ent.TenantDomain {
	now := time.Now()
	return &ent.TenantDomain{Hostname: host, IsPrimary: primary, VerifiedAt: &now}
}

func TestTenantBaseURL(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		domains TenantDomainLister
		base    string
		want    string
	}{
		{
			name: "primary verified wins",
			domains: &fakeDomainLister{rows: []*ent.TenantDomain{
				verified("first.example.ch", false),
				verified("shop.example.ch", true),
			}},
			base: "https://comanda.leys.ch",
			want: "https://shop.example.ch",
		},
		{
			name: "first verified when no primary",
			domains: &fakeDomainLister{rows: []*ent.TenantDomain{
				{Hostname: "pending.example.ch"},
				verified("first.example.ch", false),
				verified("second.example.ch", false),
			}},
			base: "https://comanda.leys.ch",
			want: "https://first.example.ch",
		},
		{
			name:    "unverified only falls back to public base",
			domains: &fakeDomainLister{rows: []*ent.TenantDomain{{Hostname: "pending.example.ch", IsPrimary: true}}},
			base:    "https://comanda.leys.ch/",
			want:    "https://comanda.leys.ch",
		},
		{
			name:    "list error falls back to public base",
			domains: &fakeDomainLister{err: errors.New("boom")},
			base:    "https://comanda.leys.ch",
			want:    "https://comanda.leys.ch",
		},
		{
			name:    "nil lister and empty base falls back to localhost",
			domains: nil,
			base:    "",
			want:    "http://localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TenantBaseURL(ctx, tt.domains, tt.base, nil); got != tt.want {
				t.Fatalf("TenantBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
