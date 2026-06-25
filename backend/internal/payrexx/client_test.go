package payrexx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestValidateCredentialsSendsSignature(t *testing.T) {
	var gotInstance, gotSig, gotAPIKeyHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotInstance = r.URL.Query().Get("instance")
		gotSig = r.URL.Query().Get("ApiSignature")
		gotAPIKeyHeader = r.Header.Get("X-API-KEY")
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
	// No params on SignatureCheck, so the signed payload is the empty string.
	if want := signParams("", "s3cr3t"); gotSig != want {
		t.Errorf("ApiSignature = %q, want %q", gotSig, want)
	}
	if gotAPIKeyHeader != "" {
		t.Errorf("X-API-KEY header should not be sent, got %q", gotAPIKeyHeader)
	}
}

func TestDoRequestPostSignsBody(t *testing.T) {
	var ct, gotSig, signedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotSig = r.PostFormValue("ApiSignature")
		// Reconstruct the signed payload: the posted form minus the signature.
		params := url.Values{}
		for k, vs := range r.PostForm {
			if k == "ApiSignature" {
				continue
			}
			for _, v := range vs {
				params.Add(k, v)
			}
		}
		signedBody = params.Encode()
		_, _ = w.Write([]byte(`{"status":"success","data":[{"id":1}]}`))
	}))
	defer srv.Close()

	c := NewClient("inst", "topsecret")
	c.baseURL = srv.URL + "/"

	if _, err := c.doRequest(context.Background(), http.MethodPost, "Gateway/", map[string]any{
		"amount":   1200,
		"currency": "CHF",
	}); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if ct != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", ct)
	}
	if want := signParams(signedBody, "topsecret"); gotSig != want {
		t.Errorf("ApiSignature = %q, want %q (over %q)", gotSig, want, signedBody)
	}
}

func TestFlattenParamsNested(t *testing.T) {
	out := url.Values{}
	flattenParams("", map[string]any{
		"amount": 1000,
		"fields": map[string]any{"email": map[string]any{"value": "a@b.ch"}},
		"pm":     []string{"visa", "mastercard"},
		"basket": []map[string]any{{"name": map[string]any{"1": "Kaffee"}, "amount": 450}},
	}, out)

	want := map[string]string{
		"amount":               "1000",
		"fields[email][value]": "a@b.ch",
		"pm[0]":                "visa",
		"pm[1]":                "mastercard",
		"basket[0][name][1]":   "Kaffee",
		"basket[0][amount]":    "450",
	}
	for k, v := range want {
		if got := out.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}
