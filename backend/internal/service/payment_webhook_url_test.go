package service

import (
	"testing"

	"backend/internal/config"
)

func TestWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			"prod apex gets api. prefix",
			"https://comanda.leys.ch",
			"https://api.comanda.leys.ch/v1/payments/webhooks/payrexx/tok",
		},
		{
			"staging apex gets api. prefix",
			"https://staging.comanda.leys.ch/",
			"https://api.staging.comanda.leys.ch/v1/payments/webhooks/payrexx/tok",
		},
		{
			"already api-prefixed host is untouched",
			"https://api.comanda.leys.ch",
			"https://api.comanda.leys.ch/v1/payments/webhooks/payrexx/tok",
		},
		{
			"localhost is untouched",
			"http://localhost:3000",
			"http://localhost:3000/v1/payments/webhooks/payrexx/tok",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{}
			cfg.App.PublicBaseURL = tc.baseURL
			s := &paymentService{cfg: cfg}
			if got := s.webhookURL("tok"); got != tc.want {
				t.Fatalf("webhookURL = %q, want %q", got, tc.want)
			}
		})
	}
}
