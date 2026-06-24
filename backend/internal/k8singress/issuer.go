package k8singress

import (
	"strings"

	"backend/internal/config"
)

// certPlan is the reconciler's decision for one verified hostname: whether to
// issue a per-domain certificate at all, and if so, with which ClusterIssuer.
type certPlan struct {
	// skip is true when a platform wildcard cert already serves the hostname, so a
	// per-domain Certificate would be redundant and burn Let's Encrypt quota.
	skip bool
	// issuer is the ClusterIssuer name to use when skip is false.
	issuer string
}

// planFor selects the TLS issuance strategy for a verified hostname:
//
//   - Covered by the *.{WildcardBase} platform wildcard -> skip (already served).
//   - At or under ManagedZone (a DNS zone our cert-manager token controls) -> the
//     DNS-01 issuer, which validates via TXT records and so works even when the
//     name is proxied through Cloudflare (HTTP-01 cannot reach the solver).
//   - Anything else (a genuine third-party domain a customer points at us) -> the
//     HTTP-01 issuer.
//
// DNS01Issuer being empty disables the DNS-01 branch, so a managed-zone host falls
// back to HTTP-01 rather than vanishing (New also rejects that misconfiguration).
func planFor(host string, cfg config.IngressConfig) certPlan {
	host = strings.ToLower(host)
	if coveredByWildcard(host, cfg.WildcardBase) {
		return certPlan{skip: true}
	}
	if cfg.DNS01Issuer != "" && underZone(host, cfg.ManagedZone) {
		return certPlan{issuer: cfg.DNS01Issuer}
	}
	return certPlan{issuer: cfg.HTTP01Issuer}
}

// coveredByWildcard reports whether host is already served by a *.{base} wildcard
// certificate, or is the base apex itself. A *.{base} wildcard matches exactly one
// label, so multi-label names below base are NOT covered and fall through.
func coveredByWildcard(host, base string) bool {
	if base == "" {
		return false
	}
	if host == base {
		return true
	}
	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	label := strings.TrimSuffix(host, suffix)
	return label != "" && !strings.Contains(label, ".")
}

func underZone(host, zone string) bool {
	if zone == "" {
		return false
	}
	return host == zone || strings.HasSuffix(host, "."+zone)
}
