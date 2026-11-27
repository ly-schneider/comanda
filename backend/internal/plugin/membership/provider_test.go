package membership

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEndpointURLRequiresHTTPSOutsideDevelopment(t *testing.T) {
	_, err := endpointURL("http://provider.example/v1", "lookup")
	require.Error(t, err)

	_, err = endpointURL("provider.example/v1", "lookup")
	require.Error(t, err, "a base without a scheme has no host to talk to")

	got, err := endpointURL("http://localhost:3000/api/benefits", "lookup")
	require.NoError(t, err, "a provider next door in development is fine")
	require.Equal(t, "http://localhost:3000/api/benefits/lookup", got)
}

func TestEndpointURLJoinsTheCallOntoTheBase(t *testing.T) {
	got, err := endpointURL("https://provider.example/apps/club/api/v1/", "redeem")
	require.NoError(t, err)
	require.Equal(t, "https://provider.example/apps/club/api/v1/redeem", got)
}

func TestProviderSendsTheKeyAndReadsTheDecision(t *testing.T) {
	var gotAuth, gotPath, gotContentType string
	var gotBody map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"status":"ok","message":"noch 1","member":{"ref":"BT42","name":"Eva Muster","groupName":"100er Club"},"entitlement":{"remaining":1,"max":1,"period":"Monat"}}`))
	}))
	defer srv.Close()

	p := &httpProvider{client: srv.Client(), logger: zap.NewNop()}
	decision, err := p.Lookup(context.Background(), Credentials{BaseURL: srv.URL, APIKey: "secret"}, "card-code")
	require.NoError(t, err)

	require.Equal(t, "Bearer secret", gotAuth)
	require.Equal(t, "application/json", gotContentType)
	require.Equal(t, "/lookup", gotPath)
	require.Equal(t, "card-code", gotBody["code"])
	require.Equal(t, StatusOK, decision.Status)
	require.Equal(t, "Eva Muster", decision.Member.Name)
	require.Equal(t, 1, decision.Entitlement.Remaining)
}

func TestProviderRedeemCarriesTheReference(t *testing.T) {
	var gotBody RedeemRequest
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"status":"ok","duplicate":true}`))
	}))
	defer srv.Close()

	p := &httpProvider{client: srv.Client(), logger: zap.NewNop()}
	decision, err := p.Redeem(context.Background(), Credentials{BaseURL: srv.URL, APIKey: "k"}, RedeemRequest{
		Reference: "ord1",
		MemberRef: "BT42",
		Quantity:  1,
		Items:     []RedeemItem{{SKU: "burger", Name: "Chicken Burger", Quantity: 1}},
	})
	require.NoError(t, err)
	require.True(t, decision.Duplicate, "a repeated booking is reported, not booked twice")
	require.Equal(t, "ord1", gotBody.Reference)
	require.Equal(t, "burger", gotBody.Items[0].SKU)
}

func TestProviderTreatsEveryNon200AsUnavailable(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := &httpProvider{client: srv.Client(), logger: zap.NewNop()}
	_, err := p.Lookup(context.Background(), Credentials{BaseURL: srv.URL, APIKey: "wrong"}, "code")
	require.ErrorIs(t, err, ErrProviderUnavailable)
}

func TestProviderRejectsAnAnswerWithoutAStatus(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":"hello"}`))
	}))
	defer srv.Close()

	p := &httpProvider{client: srv.Client(), logger: zap.NewNop()}
	_, err := p.Lookup(context.Background(), Credentials{BaseURL: srv.URL, APIKey: "k"}, "code")
	require.ErrorIs(t, err, ErrProviderUnavailable)
}

func TestProviderWithoutCredentialsNeverCallsOut(t *testing.T) {
	p := NewProvider(zap.NewNop())
	_, err := p.Lookup(context.Background(), Credentials{}, "code")
	require.ErrorIs(t, err, ErrProviderUnavailable)
}
