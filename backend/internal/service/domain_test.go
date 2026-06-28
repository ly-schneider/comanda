package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/generated/ent"
	"backend/internal/repository"
)

// fakeDomainRepo is a minimal TenantDomainRepository for service-level tests
// that don't need a database: it serves one preloaded row and records deletes.
type fakeDomainRepo struct {
	row     *ent.TenantDomain
	list    []*ent.TenantDomain
	getErr  error
	deleted bool
}

func (f *fakeDomainRepo) GetByID(context.Context, string) (*ent.TenantDomain, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.row, nil
}
func (f *fakeDomainRepo) Delete(context.Context, string) error {
	f.deleted = true
	return nil
}
func (f *fakeDomainRepo) Create(context.Context, string, string) (*ent.TenantDomain, error) {
	return f.row, nil
}
func (f *fakeDomainRepo) List(context.Context) ([]*ent.TenantDomain, error) { return f.list, nil }
func (f *fakeDomainRepo) SetPrimary(context.Context, string) error          { return nil }
func (f *fakeDomainRepo) MarkVerified(context.Context, string) (*ent.TenantDomain, error) {
	return f.row, nil
}
func (f *fakeDomainRepo) ListUnverified(context.Context, *ent.Client) ([]*ent.TenantDomain, error) {
	return nil, nil
}
func (f *fakeDomainRepo) ListVerified(context.Context, *ent.Client) ([]*ent.TenantDomain, error) {
	return nil, nil
}
func (f *fakeDomainRepo) DeleteStaleUnverified(context.Context, *ent.Client, time.Time) (int, error) {
	return 0, nil
}
func (f *fakeDomainRepo) ListAllVerifiedHostnames(context.Context, *ent.Client) ([]string, error) {
	return nil, nil
}

func newServiceWithRepo(repo repository.TenantDomainRepository, cnameTarget string) *domainService {
	cfg := config.Config{}
	cfg.CustomDomain.CNAMETarget = cnameTarget
	return &domainService{domains: repo, cfg: cfg}
}

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
		"localhost",          // single label, no dot
		"127.0.0.1",          // IP literal, not issuable by a public CA
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

func TestDeletePlatformDomainRejected(t *testing.T) {
	repo := &fakeDomainRepo{row: &ent.TenantDomain{ID: "d1", Hostname: "shop.example.com", VerificationToken: ""}}
	svc := newServiceWithRepo(repo, "ingress.comanda.leys.ch")

	if err := svc.Delete(context.Background(), "d1"); !errors.Is(err, ErrPlatformDomain) {
		t.Fatalf("Delete platform domain = %v, want ErrPlatformDomain", err)
	}
	if repo.deleted {
		t.Error("platform domain must not be deleted")
	}
}

func TestDeleteBYODomainAllowed(t *testing.T) {
	repo := &fakeDomainRepo{row: &ent.TenantDomain{ID: "d2", Hostname: "shop.example.com", VerificationToken: "tok"}}
	svc := newServiceWithRepo(repo, "ingress.comanda.leys.ch")

	if err := svc.Delete(context.Background(), "d2"); err != nil {
		t.Fatalf("Delete BYO domain = %v, want nil", err)
	}
	if !repo.deleted {
		t.Error("BYO domain should be deleted")
	}
}

func TestGetViewBuildsInstructions(t *testing.T) {
	repo := &fakeDomainRepo{row: &ent.TenantDomain{ID: "d3", Hostname: "shop.example.com", VerificationToken: "tok-abc"}}
	svc := newServiceWithRepo(repo, "ingress.comanda.leys.ch")

	view, err := svc.GetView(context.Background(), "d3")
	if err != nil {
		t.Fatalf("GetView = %v, want nil", err)
	}
	if view.CNAMETarget != "ingress.comanda.leys.ch" {
		t.Errorf("CNAMETarget = %q", view.CNAMETarget)
	}
	if view.TXTName != "_comanda-verify.shop.example.com" {
		t.Errorf("TXTName = %q", view.TXTName)
	}
	if view.TXTValue != "tok-abc" {
		t.Errorf("TXTValue = %q", view.TXTValue)
	}
}
