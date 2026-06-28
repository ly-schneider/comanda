package service

import (
	"context"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/generated/ent"

	"go.uber.org/zap"
)

func TestBuildInviteURL(t *testing.T) {
	verified := time.Now()
	cfg := config.Config{}
	cfg.App.PublicBaseURL = "https://comanda.leys.ch"

	newSvc := func(domains []*ent.TenantDomain) *adminInviteService {
		return &adminInviteService{cfg: cfg, tenantDomainRepo: &fakeDomainRepo{list: domains}, logger: zap.NewNop()}
	}

	cases := []struct {
		name    string
		domains []*ent.TenantDomain
		want    string
	}{
		{
			"verified primary wins over verified non-primary",
			[]*ent.TenantDomain{
				{Hostname: "secondary.ch", VerifiedAt: &verified, IsPrimary: false},
				{Hostname: "primary.ch", VerifiedAt: &verified, IsPrimary: true},
			},
			"https://primary.ch/invite/accept?token=tok",
		},
		{
			"first verified when none is primary",
			[]*ent.TenantDomain{
				{Hostname: "first.ch", VerifiedAt: &verified, IsPrimary: false},
				{Hostname: "second.ch", VerifiedAt: &verified, IsPrimary: false},
			},
			"https://first.ch/invite/accept?token=tok",
		},
		{
			"platform base when no domain is verified",
			[]*ent.TenantDomain{
				{Hostname: "unverified.ch", VerifiedAt: nil, IsPrimary: true},
			},
			"https://comanda.leys.ch/invite/accept?token=tok",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := newSvc(tc.domains).buildInviteURL(context.Background(), "tok")
			if got != tc.want {
				t.Fatalf("buildInviteURL = %q, want %q", got, tc.want)
			}
		})
	}
}
