package payrexx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiBaseURL = "https://api.payrexx.com/v1.0/"

const DefaultRequestTimeout = 8 * time.Second

// RefundRequestTimeout is far longer than DefaultRequestTimeout because Payrexx's
// Transaction refund endpoint regularly responds well past 20s (observed ~22.7s in
// prod). At the default 8s it times out, the request tx rolls back, and the operator
// sees a 502 even though Payrexx applied the refund. Kept under httpClient.Timeout
// (30s) so the ctx deadline governs and the call never outlives the client's cap.
const RefundRequestTimeout = 28 * time.Second

type Client struct {
	instanceName string
	apiSecret    string // sent as the X-API-KEY header
	baseURL      string
	httpClient   *http.Client
}

// NormalizeInstance reduces a user-entered instance to the bare subdomain label
// the API's `instance` parameter expects. Payrexx answers an unresolvable
// instance with an opaque HTTP 422 (a wrong API key returns 401), so a value
// like "blessthun.payrexx.com", "https://blessthun.payrexx.com/", or one with
// stray whitespace fails validation in a way that looks like bad credentials.
func NormalizeInstance(instance string) string {
	s := strings.TrimSpace(instance)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func NewClient(instanceName, apiSecret string) *Client {
	instanceName = NormalizeInstance(instanceName)
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Client{
		instanceName: instanceName,
		apiSecret:    apiSecret,
		baseURL:      apiBaseURL,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// ValidateCredentials confirms the instance + API secret authenticate, used
// before encrypting and storing per-tenant credentials.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	if c.instanceName == "" || c.apiSecret == "" {
		return fmt.Errorf("payrexx: missing instance or api secret")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultRequestTimeout)
		defer cancel()
	}
	_, err := c.doRequest(ctx, http.MethodGet, "SignatureCheck/", nil)
	return err
}

func (c *Client) doRequest(ctx context.Context, method, endpoint string, params map[string]any) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultRequestTimeout)
		defer cancel()
	}

	reqURL := c.baseURL + endpoint + "?instance=" + url.QueryEscape(c.instanceName)

	var bodyReader io.Reader
	if params != nil && (method == http.MethodPost || method == http.MethodPut) {
		payload, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("payrexx: failed to encode request: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("payrexx: failed to create request: %w", err)
	}

	req.Header.Set("X-API-KEY", c.apiSecret)
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payrexx: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("payrexx: failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("payrexx: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
