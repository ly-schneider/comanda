package service

import (
	"context"
	"net"
	"testing"

	"backend/internal/config"
	"backend/internal/repository"
)

func newTestDomainService(base, cnameTarget string) *domainService {
	cfg := config.Config{}
	cfg.CustomDomain.BaseDomain = base
	cfg.CustomDomain.CNAMETarget = cnameTarget
	return &domainService{
		domains:  repository.NewTenantDomainRepository(nil),
		cfg:      cfg,
		resolver: nil,
	}
}

func TestNormalizeHostname(t *testing.T) {
	cases := map[string]string{
		"  Shop.Example.COM ": "shop.example.com",
		"shop.example.com.":   "shop.example.com",
		"  EXAMPLE.com.  ":    "example.com",
		"already.normal.test": "already.normal.test",
	}
	for in, want := range cases {
		if got := normalizeHostname(in); got != want {
			t.Errorf("normalizeHostname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateHostname(t *testing.T) {
	svc := newTestDomainService("comanda.leys.ch", "ingress.comanda.leys.ch")

	valid := []string{
		"shop.example.com",
		"a.b.co",
		"my-shop.example.org",
		"deep.sub.domain.example.com",
	}
	for _, h := range valid {
		if err := svc.validateHostname(h); err != nil {
			t.Errorf("validateHostname(%q) = %v, want nil", h, err)
		}
	}

	invalid := []string{
		"",                   // empty
		"nodot",              // single label
		"-bad.example.com",   // leading hyphen
		"bad-.example.com",   // trailing hyphen
		"sp ace.example.com", // space
		"under_score.test",   // underscore not allowed in a hostname label
	}
	for _, h := range invalid {
		if err := svc.validateHostname(h); err != ErrInvalidHostname {
			t.Errorf("validateHostname(%q) = %v, want ErrInvalidHostname", h, err)
		}
	}

	platform := []string{
		"comanda.leys.ch",
		"foo.comanda.leys.ch",
		"deep.foo.comanda.leys.ch",
	}
	for _, h := range platform {
		if err := svc.validateHostname(h); err != ErrPlatformSubdomain {
			t.Errorf("validateHostname(%q) = %v, want ErrPlatformSubdomain", h, err)
		}
	}

	// A hostname that merely shares a suffix string but not a label boundary is
	// NOT a platform subdomain (notcomanda.leys.ch != *.comanda.leys.ch).
	if err := svc.validateHostname("notcomanda.leys.ch"); err != nil {
		t.Errorf("validateHostname(notcomanda.leys.ch) = %v, want nil", err)
	}
}

func TestValidateHostnameNoBaseDomain(t *testing.T) {
	svc := newTestDomainService("", "")
	// With no base domain configured, nothing is rejected as a platform subdomain.
	if err := svc.validateHostname("anything.example.com"); err != nil {
		t.Errorf("validateHostname with empty base = %v, want nil", err)
	}
}

func TestNewVerificationToken(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		tok, err := newVerificationToken()
		if err != nil {
			t.Fatalf("newVerificationToken: %v", err)
		}
		if len(tok) < 32 {
			t.Errorf("token too short: %q", tok)
		}
		for _, c := range tok {
			isURLSafe := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
				(c >= '0' && c <= '9') || c == '-' || c == '_'
			if !isURLSafe {
				t.Errorf("token has non-url-safe char %q in %q", c, tok)
			}
		}
		if _, dup := seen[tok]; dup {
			t.Errorf("duplicate token generated: %q", tok)
		}
		seen[tok] = struct{}{}
	}
}

// mockResolver is a table-test DNSResolver.
type mockResolver struct {
	cname    string
	cnameErr error
	txts     []string
	txtErr   error
}

func (m mockResolver) LookupCNAME(_ context.Context, _ string) (string, error) {
	return m.cname, m.cnameErr
}

func (m mockResolver) LookupTXT(_ context.Context, _ string) ([]string, error) {
	return m.txts, m.txtErr
}

func notFoundErr() error {
	return &net.DNSError{Err: "no such host", IsNotFound: true}
}

func TestDomainVerified(t *testing.T) {
	const target = "ingress.comanda.leys.ch"
	const token = "tok123"

	cases := []struct {
		name     string
		resolver mockResolver
		target   string
		token    string
		want     bool
		wantErr  bool
	}{
		{
			name:     "both records present and matching",
			resolver: mockResolver{cname: "ingress.comanda.leys.ch.", txts: []string{token}},
			target:   target, token: token, want: true,
		},
		{
			name:     "cname under target counts",
			resolver: mockResolver{cname: "edge.ingress.comanda.leys.ch.", txts: []string{token}},
			target:   target, token: token, want: true,
		},
		{
			name:     "cname matches but txt missing",
			resolver: mockResolver{cname: "ingress.comanda.leys.ch.", txtErr: notFoundErr()},
			target:   target, token: token, want: false,
		},
		{
			name:     "cname matches but txt mismatched",
			resolver: mockResolver{cname: "ingress.comanda.leys.ch.", txts: []string{"wrong"}},
			target:   target, token: token, want: false,
		},
		{
			name:     "cname mismatched even with correct txt",
			resolver: mockResolver{cname: "elsewhere.example.net.", txts: []string{token}},
			target:   target, token: token, want: false,
		},
		{
			name:     "cname not found",
			resolver: mockResolver{cnameErr: notFoundErr(), txts: []string{token}},
			target:   target, token: token, want: false,
		},
		{
			name:     "empty target disables verification",
			resolver: mockResolver{cname: "ingress.comanda.leys.ch.", txts: []string{token}},
			target:   "", token: token, want: false,
		},
		{
			name:     "empty token disables verification",
			resolver: mockResolver{cname: "ingress.comanda.leys.ch.", txts: []string{""}},
			target:   target, token: "", want: false,
		},
		{
			name:     "transient cname error surfaces",
			resolver: mockResolver{cnameErr: &net.DNSError{Err: "server misbehaving", IsTemporary: true}},
			target:   target, token: token, wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domainVerified(context.Background(), tc.resolver, "shop.example.com", tc.token, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("domainVerified = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCnameMatches(t *testing.T) {
	cases := []struct {
		resolved, target string
		want             bool
	}{
		{"ingress.comanda.leys.ch.", "ingress.comanda.leys.ch", true},
		{"INGRESS.Comanda.Leys.CH", "ingress.comanda.leys.ch", true},
		{"edge.ingress.comanda.leys.ch", "ingress.comanda.leys.ch", true},
		{"notingress.comanda.leys.ch", "ingress.comanda.leys.ch", false},
		{"elsewhere.net", "ingress.comanda.leys.ch", false},
		{"anything", "", false},
	}
	for _, c := range cases {
		if got := cnameMatches(c.resolved, c.target); got != c.want {
			t.Errorf("cnameMatches(%q,%q) = %v, want %v", c.resolved, c.target, got, c.want)
		}
	}
}
