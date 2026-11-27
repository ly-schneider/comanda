package membership

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// The membership provider protocol. A tenant points the plugin at any system
// that holds its members — a church database, a club's member list, a loyalty
// app — and that system answers three calls:
//
//	POST {base}/lookup  {code}                      -> who is this, how much is left
//	POST {base}/redeem  {reference, memberRef, ...} -> take it off their budget
//	POST {base}/void    {reference}                 -> give it back
//	POST {base}/catalog {items}                     -> here is what we sell
//	PUT  {base}/catalog {freeSkus}                  -> an admin picked these here
//
// Comanda never interprets what a member is entitled to or counts anything: the
// provider owns the rules, the quota and the period. `reference` is the order
// id, so a redeem may be retried after a timeout and a void needs nothing else.
//
// Which products are given away belongs to the provider too — it is the same
// question as how many. Only Comanda knows its catalog, so it reports it and
// gets the answer back as `program`, which rides along on every call.
const (
	// StatusOK means the call did what it says: the member may redeem, the
	// redemption is booked, or the booking is released.
	StatusOK = "ok"
	// StatusExhausted means a known member whose budget is used up.
	StatusExhausted = "exhausted"
	// StatusIneligible means a known member who may not redeem at all —
	// lapsed, blocked, or without a budget. The provider never says why.
	StatusIneligible = "ineligible"
	// StatusUnknown means the code belongs to no member, or the reference to
	// no booking.
	StatusUnknown = "unknown"
	// StatusConflict means the reference was already used for a different
	// redemption. Comanda sends order ids, so this is a bug, not a retry.
	StatusConflict = "conflict"
	// StatusVoided means the reference was released and cannot be booked again.
	StatusVoided = "voided"
	// StatusAlreadyVoided answers a second void of the same reference.
	StatusAlreadyVoided = "already_voided"
)

// ErrProviderUnavailable marks a call that never reached a decision: the
// provider was unreachable, too slow, or answered something unusable. Callers
// tell it apart from a provider that decided "no".
var ErrProviderUnavailable = errors.New("membership provider unavailable")

type Member struct {
	// Ref identifies the member to the provider and is sent back on redeem.
	// It is opaque here: never parsed, never shown to a guest.
	Ref  string `json:"ref"`
	Name string `json:"name"`
	// GroupName names the membership for the cashier, e.g. "100er Club".
	GroupName string `json:"groupName"`
}

type Entitlement struct {
	Remaining int `json:"remaining"`
	Max       int `json:"max"`
	// Period is the provider's own wording for how often the budget renews,
	// e.g. "Monat". Shown as given, never parsed.
	Period string `json:"period"`
}

// Program is what the provider gives away and what it calls the offer. It
// comes back on every call, so a till that caches it is never more than one
// scan out of date.
type Program struct {
	Label string `json:"label"`
	// FreeSkus are our own product ids, as reported by the catalog call.
	FreeSkus []string `json:"freeSkus"`
}

type Decision struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	// Duplicate marks a redeem that was already booked under this reference.
	Duplicate   bool         `json:"duplicate"`
	Member      *Member      `json:"member"`
	Entitlement *Entitlement `json:"entitlement"`
	Program     *Program     `json:"program"`
}

// CatalogItem is one thing this tenant sells, as the provider gets to see it.
type CatalogItem struct {
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	PriceCents int64  `json:"priceCents"`
}

func (d Decision) ok() bool { return d.Status == StatusOK }

// RedeemItem names what the guest is getting, for the provider's own record.
type RedeemItem struct {
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type RedeemRequest struct {
	Reference string       `json:"reference"`
	MemberRef string       `json:"memberRef"`
	Quantity  int          `json:"quantity"`
	Items     []RedeemItem `json:"items,omitempty"`
}

type Provider interface {
	Lookup(ctx context.Context, cfg Credentials, code string) (Decision, error)
	Redeem(ctx context.Context, cfg Credentials, in RedeemRequest) (Decision, error)
	Void(ctx context.Context, cfg Credentials, reference string) (Decision, error)
	// Catalog reports what this tenant sells and returns what is free of it.
	Catalog(ctx context.Context, cfg Credentials, items []CatalogItem) (Decision, error)
	// SetFree hands the provider a selection made in Comanda's own admin.
	SetFree(ctx context.Context, cfg Credentials, skus []string) (Decision, error)
}

// Credentials are the per-tenant provider secrets.
type Credentials struct {
	BaseURL string
	APIKey  string
}

func (c Credentials) configured() bool { return c.BaseURL != "" && c.APIKey != "" }

// lookupTimeout is short: a cashier is holding a card in front of a camera and
// a stalled till is worse than a missed benefit.
const (
	lookupTimeout = 5 * time.Second
	writeTimeout  = 10 * time.Second
	maxBodyBytes  = 1 << 20
)

type httpProvider struct {
	client *http.Client
	logger *zap.Logger
}

func NewProvider(logger *zap.Logger) Provider {
	return &httpProvider{
		client: &http.Client{Timeout: writeTimeout},
		logger: logger,
	}
}

func (p *httpProvider) Lookup(ctx context.Context, cfg Credentials, code string) (Decision, error) {
	return p.call(ctx, cfg, http.MethodPost, "lookup", map[string]string{"code": code}, lookupTimeout)
}

func (p *httpProvider) Redeem(ctx context.Context, cfg Credentials, in RedeemRequest) (Decision, error) {
	return p.call(ctx, cfg, http.MethodPost, "redeem", in, writeTimeout)
}

func (p *httpProvider) Void(ctx context.Context, cfg Credentials, reference string) (Decision, error) {
	return p.call(ctx, cfg, http.MethodPost, "void", map[string]string{"reference": reference}, writeTimeout)
}

func (p *httpProvider) Catalog(ctx context.Context, cfg Credentials, items []CatalogItem) (Decision, error) {
	if items == nil {
		items = []CatalogItem{}
	}
	return p.call(ctx, cfg, http.MethodPost, "catalog", map[string]any{"items": items}, writeTimeout)
}

func (p *httpProvider) SetFree(ctx context.Context, cfg Credentials, skus []string) (Decision, error) {
	if skus == nil {
		skus = []string{}
	}
	return p.call(ctx, cfg, http.MethodPut, "catalog", map[string]any{"freeSkus": skus}, writeTimeout)
}

func (p *httpProvider) call(ctx context.Context, cfg Credentials, method, path string, payload any, timeout time.Duration) (Decision, error) {
	if !cfg.configured() {
		return Decision{}, fmt.Errorf("%w: not configured", ErrProviderUnavailable)
	}
	endpoint, err := endpointURL(cfg.BaseURL, path)
	if err != nil {
		return Decision{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Decision{}, fmt.Errorf("%w: encode request: %v", ErrProviderUnavailable, err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return Decision{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		// The URL can carry the key in a misconfigured setup, so log the path only.
		p.logger.Warn("membership provider request failed", zap.String("call", path), zap.Error(err))
		return Decision{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		p.logger.Warn("membership provider returned an error",
			zap.String("call", path),
			zap.Int("status", resp.StatusCode),
		)
		return Decision{}, fmt.Errorf("%w: status %d", ErrProviderUnavailable, resp.StatusCode)
	}

	var decision Decision
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&decision); err != nil {
		p.logger.Warn("membership provider sent an unreadable answer", zap.String("call", path), zap.Error(err))
		return Decision{}, fmt.Errorf("%w: decode response: %v", ErrProviderUnavailable, err)
	}
	if decision.Status == "" {
		return Decision{}, fmt.Errorf("%w: answer without a status", ErrProviderUnavailable)
	}
	return decision, nil
}

// endpointURL joins the configured base with a call name. Only https is allowed
// (a member's card code travels in the body), except on localhost, so a tenant
// can point the plugin at a provider running next to it in development.
func endpointURL(base, path string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", fmt.Errorf("invalid base url")
	}
	if u.Host == "" {
		return "", fmt.Errorf("base url without a host")
	}
	if u.Scheme != "https" && !isLoopback(u.Hostname()) {
		return "", fmt.Errorf("base url must be https")
	}
	u.Path = u.Path + "/" + path
	return u.String(), nil
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
