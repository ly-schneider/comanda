package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"strings"

	"backend/internal/config"
	"backend/internal/dnsname"
	"backend/internal/generated/ent"
	"backend/internal/repository"
)

var (
	// ErrInvalidHostname signals a 400: the hostname is not a plausible FQDN.
	ErrInvalidHostname = errors.New("invalid_hostname")
	// ErrPlatformSubdomain signals a 400: the hostname is under the platform's
	// own base domain, so it is a platform subdomain, not a BYO custom domain.
	ErrPlatformSubdomain = errors.New("platform_subdomain")
	// ErrPlatformDomain signals a 400: the row is a platform-assigned domain
	// (seeded, no verification token) and must not be removed by a tenant admin,
	// since deleting it would route all the tenant's traffic to a 404.
	ErrPlatformDomain = errors.New("platform_domain")
	// ErrDomainNotVerified signals a 400: an unverified domain cannot be promoted
	// to the standard domain, as generated links would point at a host that does
	// not yet route to the tenant.
	ErrDomainNotVerified = errors.New("domain_not_verified")
)

// verifyTXTPrefix is the DNS label the ownership TXT record lives under:
// _comanda-verify.<hostname> -> <verification_token>.
const verifyTXTPrefix = "_comanda-verify."

// DNSResolver is the subset of DNS lookups the verifier needs. Factored behind
// an interface so the ownership decision is unit-testable without real DNS.
type DNSResolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

type netResolver struct {
	r *net.Resolver
}

// NewNetResolver returns the production DNSResolver backed by the system resolver.
func NewNetResolver() DNSResolver {
	return &netResolver{r: net.DefaultResolver}
}

func (n *netResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	return n.r.LookupCNAME(ctx, host)
}

func (n *netResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return n.r.LookupTXT(ctx, name)
}

// DomainView is the API-facing representation of a custom domain plus the DNS
// instructions the tenant must publish to verify ownership.
type DomainView struct {
	Domain *ent.TenantDomain
	// CNAMETarget is the value the tenant points the domain's CNAME at.
	CNAMETarget string
	// TXTName is the record name the verification TXT lives at.
	TXTName string
	// TXTValue is the verification token to publish.
	TXTValue string
}

type DomainService interface {
	Create(ctx context.Context, hostname string) (*DomainView, error)
	List(ctx context.Context) ([]*ent.TenantDomain, error)
	// GetView loads one of the tenant's own domains and enriches it with its DNS
	// instructions, so an admin can recover the CNAME/TXT records after navigating
	// away from the create response.
	GetView(ctx context.Context, id string) (*DomainView, error)
	Delete(ctx context.Context, id string) error
	SetPrimary(ctx context.Context, id string) error
	// VerifyNow runs the CNAME+TXT ownership check for one domain immediately and
	// marks it verified on success.
	VerifyNow(ctx context.Context, id string) (*ent.TenantDomain, error)
	// View enriches a stored domain with its DNS instructions for the API.
	View(d *ent.TenantDomain) *DomainView
	// ListAllVerifiedHostnames returns every verified hostname across all tenants.
	// The caller passes the privileged (RLS-bypassing) client; this is the
	// cross-tenant trusted-origins feed and never goes through a tenant tx.
	ListAllVerifiedHostnames(ctx context.Context, privileged *ent.Client) ([]string, error)
}

type domainService struct {
	domains  repository.TenantDomainRepository
	cfg      config.Config
	resolver DNSResolver
}

func NewDomainService(domains repository.TenantDomainRepository, cfg config.Config) DomainService {
	return &domainService{domains: domains, cfg: cfg, resolver: NewNetResolver()}
}

// normalizeHostname lowercases, trims, and strips a trailing dot.
func normalizeHostname(raw string) string {
	h := strings.ToLower(strings.TrimSpace(raw))
	h = strings.TrimSuffix(h, ".")
	return h
}

// validateHostname checks the hostname is a plausible FQDN and not under the
// platform base domain.
func (s *domainService) validateHostname(hostname string) error {
	if !dnsname.IsPublicFQDN(hostname) {
		return ErrInvalidHostname
	}
	if base := s.cfg.CustomDomain.BaseDomain; base != "" {
		if hostname == base || strings.HasSuffix(hostname, "."+base) {
			return ErrPlatformSubdomain
		}
	}
	return nil
}

// newVerificationToken returns a url-safe random token for the ownership TXT
// record.
func newVerificationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *domainService) Create(ctx context.Context, hostname string) (*DomainView, error) {
	hostname = normalizeHostname(hostname)
	if err := s.validateHostname(hostname); err != nil {
		return nil, err
	}

	token, err := newVerificationToken()
	if err != nil {
		return nil, err
	}

	created, err := s.domains.Create(ctx, hostname, token)
	if err != nil {
		return nil, err
	}
	return s.View(created), nil
}

func (s *domainService) List(ctx context.Context) ([]*ent.TenantDomain, error) {
	return s.domains.List(ctx)
}

func (s *domainService) GetView(ctx context.Context, id string) (*DomainView, error) {
	d, err := s.domains.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.View(d), nil
}

func (s *domainService) Delete(ctx context.Context, id string) error {
	d, err := s.domains.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// Platform-assigned domains are seeded without a verification token; removing
	// one would point the tenant's traffic at a 404 and lock them out.
	if d.VerificationToken == "" {
		return ErrPlatformDomain
	}
	return s.domains.Delete(ctx, id)
}

func (s *domainService) SetPrimary(ctx context.Context, id string) error {
	d, err := s.domains.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if d.VerifiedAt == nil {
		return ErrDomainNotVerified
	}
	return s.domains.SetPrimary(ctx, id)
}

func (s *domainService) VerifyNow(ctx context.Context, id string) (*ent.TenantDomain, error) {
	d, err := s.domains.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Re-checking an already-verified domain is intentionally a no-op: there is no
	// downgrade/revocation path, so the manual verify button can never un-verify a
	// domain (e.g. on a transient DNS blip).
	if d.VerifiedAt != nil {
		return d, nil
	}
	ok, err := domainVerified(ctx, s.resolver, d.Hostname, d.VerificationToken, s.cfg.CustomDomain.CNAMETarget)
	if err != nil {
		return nil, err
	}
	if !ok {
		return d, nil
	}
	return s.domains.MarkVerified(ctx, id)
}

func (s *domainService) ListAllVerifiedHostnames(ctx context.Context, privileged *ent.Client) ([]string, error) {
	return s.domains.ListAllVerifiedHostnames(ctx, privileged)
}

func (s *domainService) View(d *ent.TenantDomain) *DomainView {
	return &DomainView{
		Domain:      d,
		CNAMETarget: s.cfg.CustomDomain.CNAMETarget,
		TXTName:     verifyTXTPrefix + d.Hostname,
		TXTValue:    d.VerificationToken,
	}
}

// domainVerified is the pure ownership decision: ownership requires BOTH the
// CNAME to resolve to the configured target AND a matching TXT token. Requiring
// the CNAME (not just the TXT) ensures traffic actually routes to the platform
// before a certificate is requested. An empty cnameTarget disables verification.
func domainVerified(ctx context.Context, resolver DNSResolver, hostname, token, cnameTarget string) (bool, error) {
	if cnameTarget == "" || token == "" {
		return false, nil
	}

	cname, err := resolver.LookupCNAME(ctx, hostname)
	if err != nil {
		if isDNSNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if !cnameMatches(cname, cnameTarget) {
		return false, nil
	}

	txts, err := resolver.LookupTXT(ctx, verifyTXTPrefix+hostname)
	if err != nil {
		if isDNSNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, txt := range txts {
		if strings.TrimSpace(txt) == token {
			return true, nil
		}
	}
	return false, nil
}

// cnameMatches reports whether the resolved CNAME points at the configured
// target. Both are compared as dot-trimmed, lowercased FQDNs; a resolved CNAME
// at or under the target counts (the target may be a base domain).
func cnameMatches(resolved, target string) bool {
	r := strings.ToLower(strings.TrimSuffix(resolved, "."))
	t := strings.ToLower(strings.TrimSuffix(target, "."))
	if t == "" {
		return false
	}
	// A sub-host under the ingress target is intentionally accepted: edge/anycast
	// records (e.g. edge.<target>) still route to the platform host.
	return r == t || strings.HasSuffix(r, "."+t)
}

// isDNSNotFound reports whether err is a benign "no such record" DNS result, as
// opposed to a transient resolver failure that should be retried later.
func isDNSNotFound(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsNotFound
	}
	return false
}
