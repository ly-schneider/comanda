// Package dnsname holds hostname validation shared by the domain service (API
// input on create) and the k8s ingress reconciler (defense-in-depth before it
// asks a public CA to issue a certificate). Keeping it a dependency-free leaf
// package lets both layers use the same rule without coupling them to each other.
package dnsname

import (
	"net"
	"regexp"
)

// fqdnPattern matches a lowercase FQDN: 2+ dot-separated labels, each 1-63
// chars of [a-z0-9-] without a leading/trailing hyphen.
var fqdnPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// IsPublicFQDN reports whether h is a plausible public fully-qualified domain
// name a public CA could issue for: a multi-label hostname that is not an IP
// literal. It does NOT check DNS resolution or registrability. Single-label
// names ("localhost") and IPs ("127.0.0.1", "::1") are rejected — Let's Encrypt
// refuses them ("Domain name needs at least one dot"), so requesting one leaves
// a Certificate stuck un-issued forever.
func IsPublicFQDN(h string) bool {
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	// 127.0.0.1 satisfies the label regex (all-numeric labels), so screen IPs out
	// explicitly before the pattern check.
	if net.ParseIP(h) != nil {
		return false
	}
	return fqdnPattern.MatchString(h)
}
