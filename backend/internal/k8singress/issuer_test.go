package k8singress

import (
	"testing"

	"backend/internal/config"
)

func TestPlanFor(t *testing.T) {
	cfg := config.IngressConfig{
		HTTP01Issuer: "letsencrypt-http01",
		DNS01Issuer:  "letsencrypt-cf-dns01",
		ManagedZone:  "comanda.leys.ch",
		WildcardBase: "staging.comanda.leys.ch",
	}
	cases := []struct {
		name     string
		host     string
		wantSkip bool
		wantIss  string
	}{
		{"wildcard subdomain is skipped", "incil.staging.comanda.leys.ch", true, ""},
		{"wildcard base apex is skipped", "staging.comanda.leys.ch", true, ""},
		{"multi-label below base uses dns01", "a.b.staging.comanda.leys.ch", false, "letsencrypt-cf-dns01"},
		{"managed-zone vanity uses dns01", "default.comanda.leys.ch", false, "letsencrypt-cf-dns01"},
		{"managed-zone apex uses dns01", "comanda.leys.ch", false, "letsencrypt-cf-dns01"},
		{"external byo uses http01", "shop.festival.ch", false, "letsencrypt-http01"},
		{"casing is normalized", "Shop.Festival.CH", false, "letsencrypt-http01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := planFor(tc.host, cfg)
			if got.skip != tc.wantSkip {
				t.Fatalf("skip = %v, want %v", got.skip, tc.wantSkip)
			}
			if !got.skip && got.issuer != tc.wantIss {
				t.Errorf("issuer = %q, want %q", got.issuer, tc.wantIss)
			}
		})
	}
}

func TestPlanForManagedZoneWithoutDNS01FallsBackToHTTP01(t *testing.T) {
	// ManagedZone set but no DNS01Issuer: a managed-zone host must not silently
	// vanish; it falls back to HTTP-01. (New rejects this combo at startup, but the
	// planner stays safe regardless.)
	cfg := config.IngressConfig{HTTP01Issuer: "letsencrypt-http01", ManagedZone: "comanda.leys.ch"}
	got := planFor("default.comanda.leys.ch", cfg)
	if got.skip || got.issuer != "letsencrypt-http01" {
		t.Errorf("got %+v, want {skip:false issuer:letsencrypt-http01}", got)
	}
}
