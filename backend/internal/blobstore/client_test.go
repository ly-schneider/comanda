package blobstore

import "testing"

func TestKeyFromURL(t *testing.T) {
	c := &Client{publicBaseURL: "https://cdn.example.com"}

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"object", "https://cdn.example.com/products/abc", "products/abc"},
		{"nested branding key", "https://cdn.example.com/tenants/t1/branding/logo-x.png", "tenants/t1/branding/logo-x.png"},
		{"foreign host returns empty", "https://other.example.com/products/abc", ""},
		{"base without key returns empty", "https://cdn.example.com/", ""},
		{"unrelated returns empty", "not-a-url", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.keyFromURL(tt.url); got != tt.want {
				t.Fatalf("keyFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	c, err := NewClient("https://acct.r2.cloudflarestorage.com", "k", "s", "bucket", "https://cdn.example.com/")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.publicBaseURL != "https://cdn.example.com" {
		t.Fatalf("publicBaseURL = %q, want trimmed", c.publicBaseURL)
	}
	if got := c.keyFromURL("https://cdn.example.com/k"); got != "k" {
		t.Fatalf("keyFromURL = %q, want k", got)
	}
}
