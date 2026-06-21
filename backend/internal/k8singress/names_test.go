package k8singress

import (
	"regexp"
	"testing"
)

// rfc1123 is the lowercase DNS label/subdomain pattern K8s resource names must
// satisfy: lowercase alnum, '-' and '.', starting and ending alphanumeric.
var rfc1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

func TestResourceNameRFC1123(t *testing.T) {
	// Worst-case nanoid: full of uppercase, underscores, and hyphens — all of
	// which are illegal (uppercase, '_') or fine but must not leak verbatim.
	ids := []string{
		"AB_cd-EF_gh1",
		"____________",
		"ZZZZZZZZZZZZ",
		"a1b2c3d4e5f6",
		"shop_DOMAIN1",
	}
	for _, id := range ids {
		name := resourceName(id)
		if !rfc1123.MatchString(name) {
			t.Errorf("resourceName(%q) = %q not RFC-1123", id, name)
		}
		if len(name) > 63 {
			t.Errorf("resourceName(%q) = %q exceeds 63 chars", id, name)
		}
		secret := tlsSecretName(id)
		if !rfc1123.MatchString(secret) {
			t.Errorf("tlsSecretName(%q) = %q not RFC-1123", id, secret)
		}
		if len(secret) > 63 {
			t.Errorf("tlsSecretName(%q) = %q exceeds 63 chars", id, secret)
		}
	}
}

func TestResourceNameDeterministicAndDistinct(t *testing.T) {
	a1 := resourceName("AB_cd-EF_gh1")
	a2 := resourceName("AB_cd-EF_gh1")
	b := resourceName("a1b2c3d4e5f6")
	if a1 != a2 {
		t.Errorf("resourceName not deterministic: %q != %q", a1, a2)
	}
	if a1 == b {
		t.Errorf("distinct ids collided: both %q", a1)
	}
}

func TestTLSSecretNameSuffix(t *testing.T) {
	id := "a1b2c3d4e5f6"
	if got, want := tlsSecretName(id), resourceName(id)+"-tls"; got != want {
		t.Errorf("tlsSecretName = %q, want %q", got, want)
	}
}
