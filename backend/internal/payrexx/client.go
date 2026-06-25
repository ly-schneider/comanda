package payrexx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBaseURL = "https://api.payrexx.com/v1.0/"

const DefaultRequestTimeout = 8 * time.Second

type Client struct {
	instanceName string
	apiSecret    string
	baseURL      string
	httpClient   *http.Client
}

func NewClient(instanceName, apiSecret string) *Client {
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

	// Payrexx v1.0 authenticates every call with an HMAC-SHA256 signature of the
	// form-encoded request parameters, keyed by the API secret and base64-encoded,
	// passed as the ApiSignature parameter. The instance name is a separate query
	// parameter and is never part of the signed payload.
	values := url.Values{}
	flattenParams("", params, values)
	values.Set("ApiSignature", signParams(values.Encode(), c.apiSecret))

	reqURL := c.baseURL + endpoint + "?instance=" + url.QueryEscape(c.instanceName)

	var req *http.Request
	var err error
	switch method {
	case http.MethodGet, http.MethodDelete:
		reqURL += "&" + values.Encode()
		req, err = http.NewRequestWithContext(ctx, method, reqURL, nil)
	default:
		req, err = http.NewRequestWithContext(ctx, method, reqURL, strings.NewReader(values.Encode()))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	if err != nil {
		return nil, fmt.Errorf("payrexx: failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

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

func signParams(query, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(query))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// flattenParams expands nested params into PHP http_build_query bracket form
// (fields[email][value], basket[0][amount], pm[0], …) so the signature we compute
// matches what Payrexx reconstructs from the parameters it receives.
func flattenParams(prefix string, value any, out url.Values) {
	switch v := value.(type) {
	case map[string]any:
		for key, val := range v {
			flattenParams(childKey(prefix, key), val, out)
		}
	case []map[string]any:
		for i, val := range v {
			flattenParams(childKey(prefix, strconv.Itoa(i)), val, out)
		}
	case []any:
		for i, val := range v {
			flattenParams(childKey(prefix, strconv.Itoa(i)), val, out)
		}
	case []string:
		for i, val := range v {
			flattenParams(childKey(prefix, strconv.Itoa(i)), val, out)
		}
	case nil:
		// Unset optional fields carry no signed value.
	default:
		out.Set(prefix, fmt.Sprintf("%v", v))
	}
}

func childKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "[" + key + "]"
}
