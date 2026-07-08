package payrexx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeInstance(t *testing.T) {
	cases := map[string]string{
		"blessthun":                              "blessthun",
		"blessthun.payrexx.com":                  "blessthun",
		"https://blessthun.payrexx.com":          "blessthun",
		"http://blessthun.payrexx.com/":          "blessthun",
		"  blessthun  ":                          "blessthun",
		"blessthun\n":                            "blessthun",
		"https://blessthun.payrexx.com/?foo=bar": "blessthun",
		"":                                       "",
		" ":                                      "",
	}
	for in, want := range cases {
		if got := NormalizeInstance(in); got != want {
			t.Errorf("NormalizeInstance(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateCredentialsSendsAPIKey(t *testing.T) {
	var gotInstance, gotSig, gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotInstance = r.URL.Query().Get("instance")
		gotSig = r.URL.Query().Get("ApiSignature")
		gotAPIKey = r.Header.Get("X-API-KEY")
		_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
	}))
	defer srv.Close()

	c := NewClient("my-instance", "s3cr3t")
	c.baseURL = srv.URL + "/"

	if err := c.ValidateCredentials(context.Background()); err != nil {
		t.Fatalf("ValidateCredentials: %v", err)
	}
	if gotInstance != "my-instance" {
		t.Errorf("instance = %q, want my-instance", gotInstance)
	}
	if gotAPIKey != "s3cr3t" {
		t.Errorf("X-API-KEY = %q, want s3cr3t", gotAPIKey)
	}
	if gotSig != "" {
		t.Errorf("ApiSignature should not be sent under X-API-KEY, got %q", gotSig)
	}
}

func TestRefundTransaction(t *testing.T) {
	cases := []struct {
		name    string
		reply   string
		status  int
		wantErr bool
	}{
		{"success", `{"status":"success","data":[{"id":1}]}`, http.StatusOK, false},
		// A Payrexx error status inside a 200 body must surface as an error.
		{"error body on 200", `{"status":"error","message":"amount too high"}`, http.StatusOK, true},
		{"http error", `{"status":"error"}`, http.StatusBadRequest, true},
		// An empty or status-less 2xx body is a success — don't fail an applied refund.
		{"empty body", ``, http.StatusOK, false},
		{"no status field", `{"data":[{"id":1}]}`, http.StatusOK, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.reply))
			}))
			defer srv.Close()

			c2 := NewClient("inst", "secret")
			c2.baseURL = srv.URL + "/"

			err := c2.RefundTransaction(context.Background(), 42, 300)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDoRequestPostSendsJSON(t *testing.T) {
	var ct, gotAPIKey string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		gotAPIKey = r.Header.Get("X-API-KEY")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"status":"success","data":[{"id":1}]}`))
	}))
	defer srv.Close()

	c := NewClient("inst", "topsecret")
	c.baseURL = srv.URL + "/"

	if _, err := c.doRequest(context.Background(), http.MethodPost, "Gateway/", map[string]any{
		"amount":   1200,
		"currency": "CHF",
		"fields":   map[string]any{"email": map[string]any{"value": "a@b.ch"}},
		"pm":       []string{"visa"},
		"basket":   []map[string]any{{"name": map[string]any{"1": "Kaffee"}, "amount": 450}},
	}); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if gotAPIKey != "topsecret" {
		t.Errorf("X-API-KEY = %q, want topsecret", gotAPIKey)
	}
	if body["currency"] != "CHF" {
		t.Errorf("currency = %v, want CHF", body["currency"])
	}

	// Nested fields/basket round-trip as JSON objects/arrays (the shape Payrexx expects).
	fields, _ := body["fields"].(map[string]any)
	email, _ := fields["email"].(map[string]any)
	if email["value"] != "a@b.ch" {
		t.Errorf("fields.email.value = %v, want a@b.ch", email["value"])
	}
	basket, _ := body["basket"].([]any)
	if len(basket) != 1 {
		t.Fatalf("basket len = %d, want 1", len(basket))
	}
	item, _ := basket[0].(map[string]any)
	name, _ := item["name"].(map[string]any)
	if name["1"] != "Kaffee" {
		t.Errorf("basket[0].name.1 = %v, want Kaffee", name["1"])
	}
}
