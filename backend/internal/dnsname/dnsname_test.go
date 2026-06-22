package dnsname

import "testing"

func TestIsPublicFQDN(t *testing.T) {
	valid := []string{
		"shop.example.com",
		"a.b.co",
		"my-shop.example.org",
		"deep.sub.domain.example.com",
	}
	for _, h := range valid {
		if !IsPublicFQDN(h) {
			t.Errorf("IsPublicFQDN(%q) = false, want true", h)
		}
	}

	invalid := []string{
		"",                   // empty
		"localhost",          // single label, no dot
		"nodot",              // single label
		"127.0.0.1",          // IPv4 literal (matches the label regex, must be screened)
		"::1",                // IPv6 literal
		"-bad.example.com",   // leading hyphen
		"bad-.example.com",   // trailing hyphen
		"sp ace.example.com", // space
		"under_score.test",   // underscore not allowed in a hostname label
	}
	for _, h := range invalid {
		if IsPublicFQDN(h) {
			t.Errorf("IsPublicFQDN(%q) = true, want false", h)
		}
	}
}
