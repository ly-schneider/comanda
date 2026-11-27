package membership

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/plugin"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"go.uber.org/zap"
)

const Key = "membership"

// defaultLabel names the benefit where a tenant has not. Shown on the till
// tile and above the guest's own redemption page.
const defaultLabel = "Mitgliederkarte"

var (
	// ErrNotEligible covers every answer that is not a yes: an unknown card, a
	// lapsed membership, an empty budget. The cashier gets the provider's
	// message; the reason never travels further than that.
	ErrNotEligible = errors.New("membership_not_eligible")
	// ErrProductNotFree marks a claim on a product the tenant does not give away.
	ErrProductNotFree = errors.New("membership_product_not_free")
	// ErrNothingClaimed marks a claim without a single unit to hand over.
	ErrNothingClaimed = errors.New("membership_nothing_claimed")
)

// Settings is what the admin screen shows. None of it is Comanda's to decide:
// the provider holds the offer's name and which products it covers, the same
// way it holds how many. Comanda only knows which of those ids still exist.
type Settings struct {
	Label          string   `json:"label"`
	FreeProductIDs []string `json:"freeProductIds"`
	// Synced is false when the provider could not be reached and these values
	// are the last ones it sent. Editing then has nowhere to go.
	Synced bool `json:"synced"`
}

// Lookup is what the till learns about a scanned card.
type Lookup struct {
	Status     string
	Message    string
	MemberRef  string
	MemberName string
	GroupName  string
	Remaining  int
	Max        int
	Period     string
}

func (l Lookup) CanRedeem() bool { return l.Status == StatusOK && l.Remaining > 0 }

// ClaimProduct is one thing a member may pick on their own phone.
type ClaimProduct struct {
	ID    string
	Name  string
	Image *string
}

// ClaimView is the guest-facing answer: who they are, what is left, what they
// can pick. It never carries the member's provider reference — the phone sends
// the card code again when it redeems.
type ClaimView struct {
	Lookup   Lookup
	Label    string
	Products []ClaimProduct
}

type ClaimItem struct {
	ProductID string
	Quantity  int
}

// ClaimResult is the minted order: the guest shows its pickup code at the stand.
type ClaimResult struct {
	OrderID   string
	QRPayload string
	Member    Lookup
}

type ProductLookup interface {
	GetByIDs(ctx context.Context, ids []string) ([]*ent.Product, error)
	FilterBuyable(ctx context.Context, ids []string) (map[string]struct{}, error)
	// GetAllVisible is the catalog Comanda reports to the provider, so an
	// admin there picks from names and prices instead of our product ids.
	GetAllVisible(ctx context.Context) ([]*ent.Product, error)
}

type StockLookup interface {
	GetCurrentStockBatch(ctx context.Context, productIDs []string) (map[string]int, error)
}

// OrderMinter creates the guest's free order the same way a paid checkout does,
// so stock, the pickup token and the receipt all behave normally.
type OrderMinter interface {
	PrepareAndCreateOrder(ctx context.Context, in coreservice.CreateCheckoutInput, userID *string, attemptID *string) (*coreservice.CheckoutPreparation, error)
}

type OrderSettler interface {
	SetPosPaymentGratisPlugin(ctx context.Context, orderID string, deviceID *string, amountCents int64, pluginKey string) error
}

type Service interface {
	GetSettings(ctx context.Context) (Settings, error)
	SetSettings(ctx context.Context, freeProductIDs []string) (Settings, error)
	Lookup(ctx context.Context, code string) (Lookup, error)
	ClaimView(ctx context.Context, code string) (ClaimView, error)
	Claim(ctx context.Context, code string, items []ClaimItem) (ClaimResult, error)
	ValidateOrderClaim(ctx context.Context, orderID string, quantity int) ([]RedeemItem, error)
	Book(ctx context.Context, orderID, memberRef, memberName string, quantity int, items []RedeemItem) error
	Release(ctx context.Context, ord *ent.Order, in plugin.RefundedOrder) error
}

// config caches the provider's answer so the till and the shop do not wait on
// a second call per scan. It is never edited here: every call that reaches the
// provider refreshes it.
type config struct {
	Label          string   `json:"label,omitempty"`
	FreeProductIDs []string `json:"freeProductIds,omitempty"`
}

func (c config) label() string {
	if c.Label == "" {
		return defaultLabel
	}
	return c.Label
}

type service struct {
	plugins     plugin.Service
	redemptions RedemptionRepository
	provider    Provider
	products    ProductLookup
	stock       StockLookup
	orderLines  repository.OrderLineRepository
	orders      OrderMinter
	settle      OrderSettler
	logger      *zap.Logger
}

func NewService(
	plugins plugin.Service,
	redemptions RedemptionRepository,
	provider Provider,
	products ProductLookup,
	stock StockLookup,
	orderLines repository.OrderLineRepository,
	orders OrderMinter,
	settle OrderSettler,
	logger *zap.Logger,
) Service {
	return &service{
		plugins:     plugins,
		redemptions: redemptions,
		provider:    provider,
		products:    products,
		stock:       stock,
		orderLines:  orderLines,
		orders:      orders,
		settle:      settle,
		logger:      logger,
	}
}

func (s *service) config(ctx context.Context) (config, error) {
	raw, err := s.plugins.Config(ctx, Key)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return config{}, fmt.Errorf("parse config: %w", err)
		}
	}
	return cfg, nil
}

func (s *service) credentials(ctx context.Context) (Credentials, error) {
	secrets, err := s.plugins.Secrets(ctx, Key)
	if err != nil {
		return Credentials{}, fmt.Errorf("read secrets: %w", err)
	}
	return Credentials{BaseURL: secrets["baseUrl"], APIKey: secrets["apiKey"]}, nil
}

// GetSettings reports this tenant's catalog to the provider and answers with
// what came back. Opening the screen is therefore also the moment the two
// sides agree again — an admin who picked products in the member system sees
// them here without anyone pressing sync.
func (s *service) GetSettings(ctx context.Context) (Settings, error) {
	items, err := s.catalogItems(ctx)
	if err != nil {
		return Settings{}, err
	}
	return s.fromProvider(ctx, func(creds Credentials) (Decision, error) {
		return s.provider.Catalog(ctx, creds, items)
	})
}

// SetSettings hands a selection made here to the provider, which owns it. What
// comes back is what is now true — not necessarily what was sent.
func (s *service) SetSettings(ctx context.Context, freeProductIDs []string) (Settings, error) {
	ids := make([]string, 0, len(freeProductIDs))
	for _, id := range freeProductIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return s.fromProvider(ctx, func(creds Credentials) (Decision, error) {
		return s.provider.SetFree(ctx, creds, ids)
	})
}

// fromProvider runs a settings call and falls back to the cache when the
// provider is unreachable: the screen still shows what is being given away,
// marked as not in sync, instead of an error page.
func (s *service) fromProvider(ctx context.Context, call func(Credentials) (Decision, error)) (Settings, error) {
	creds, err := s.credentials(ctx)
	if err != nil {
		return Settings{}, err
	}
	decision, callErr := call(creds)
	if callErr == nil && decision.Program != nil {
		if err := s.adoptProgram(ctx, *decision.Program); err != nil {
			return Settings{}, err
		}
	}
	cfg, err := s.config(ctx)
	if err != nil {
		return Settings{}, err
	}
	ids, err := s.existingProductIDs(ctx, cfg.FreeProductIDs)
	if err != nil {
		return Settings{}, err
	}
	if callErr != nil {
		s.logger.Warn("membership provider unreachable, showing the cached offer", zap.Error(callErr))
	}
	return Settings{Label: cfg.label(), FreeProductIDs: ids, Synced: callErr == nil}, nil
}

// catalogItems is what this tenant sells, in the provider's words. Deactivated
// and hidden products stay out: nobody should pick something the stand does
// not offer.
func (s *service) catalogItems(ctx context.Context) ([]CatalogItem, error) {
	products, err := s.products.GetAllVisible(ctx)
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}
	items := make([]CatalogItem, 0, len(products))
	for _, p := range products {
		items = append(items, CatalogItem{SKU: p.ID, Name: p.Name, PriceCents: p.PriceCents})
	}
	return items, nil
}

// adoptProgram writes the provider's answer into the cache, and only when it
// changed — a scan at a busy till must not write a row every time.
func (s *service) adoptProgram(ctx context.Context, program Program) error {
	cfg, err := s.config(ctx)
	if err != nil {
		return err
	}
	if cfg.Label == program.Label && slices.Equal(cfg.FreeProductIDs, program.FreeSkus) {
		return nil
	}
	cfg.Label = program.Label
	cfg.FreeProductIDs = program.FreeSkus
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if _, err := s.plugins.SetConfig(ctx, Key, raw); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// rememberProgram keeps the cache current from an ordinary scan. It never
// fails a redemption: a guest waiting at the till does not care that a label
// is a minute old.
func (s *service) rememberProgram(ctx context.Context, program *Program) {
	if program == nil {
		return
	}
	if err := s.adoptProgram(ctx, *program); err != nil {
		s.logger.Warn("could not store the membership offer", zap.Error(err))
	}
}

func (s *service) Lookup(ctx context.Context, code string) (Lookup, error) {
	creds, err := s.credentials(ctx)
	if err != nil {
		return Lookup{}, err
	}
	decision, err := s.provider.Lookup(ctx, creds, code)
	if err != nil {
		return Lookup{}, err
	}
	s.rememberProgram(ctx, decision.Program)
	return toLookup(decision), nil
}

func toLookup(d Decision) Lookup {
	out := Lookup{Status: d.Status, Message: d.Message}
	if d.Member != nil {
		out.MemberRef = d.Member.Ref
		out.MemberName = d.Member.Name
		out.GroupName = d.Member.GroupName
	}
	if d.Entitlement != nil {
		out.Remaining = d.Entitlement.Remaining
		out.Max = d.Entitlement.Max
		out.Period = d.Entitlement.Period
	}
	return out
}

func (s *service) ClaimView(ctx context.Context, code string) (ClaimView, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return ClaimView{}, err
	}
	look, err := s.Lookup(ctx, code)
	if err != nil {
		return ClaimView{}, err
	}
	view := ClaimView{Lookup: look, Label: cfg.label(), Products: []ClaimProduct{}}
	// The member's reference stays on the server: the phone proves who it is
	// with the card code, which it already has.
	view.Lookup.MemberRef = ""
	if !look.CanRedeem() {
		return view, nil
	}
	products, err := s.availableProducts(ctx, cfg.FreeProductIDs)
	if err != nil {
		return ClaimView{}, err
	}
	view.Products = products
	return view, nil
}

// availableProducts keeps the picker honest: deactivated products and empty
// stock are left out, so a guest never claims something the stand cannot hand over.
func (s *service) availableProducts(ctx context.Context, ids []string) ([]ClaimProduct, error) {
	if len(ids) == 0 {
		return []ClaimProduct{}, nil
	}
	products, err := s.products.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load products: %w", err)
	}
	buyable, err := s.products.FilterBuyable(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("check availability: %w", err)
	}
	stock, err := s.stock.GetCurrentStockBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("check stock: %w", err)
	}

	byID := make(map[string]*ent.Product, len(products))
	for _, p := range products {
		byID[p.ID] = p
	}
	out := make([]ClaimProduct, 0, len(ids))
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			continue
		}
		if _, ok := buyable[id]; !ok {
			continue
		}
		if stock[id] <= 0 {
			continue
		}
		out = append(out, ClaimProduct{ID: p.ID, Name: p.Name, Image: p.Image})
	}
	return out, nil
}

func (s *service) Claim(ctx context.Context, code string, items []ClaimItem) (ClaimResult, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return ClaimResult{}, err
	}
	free := idSet(cfg.FreeProductIDs)

	quantity := 0
	for _, it := range items {
		if it.Quantity <= 0 {
			return ClaimResult{}, fmt.Errorf("%w: quantity must be positive", ErrNothingClaimed)
		}
		if !free[it.ProductID] {
			return ClaimResult{}, ErrProductNotFree
		}
		quantity += it.Quantity
	}
	if quantity == 0 {
		return ClaimResult{}, ErrNothingClaimed
	}

	look, err := s.Lookup(ctx, code)
	if err != nil {
		return ClaimResult{}, err
	}
	if !look.CanRedeem() || look.Remaining < quantity {
		return ClaimResult{}, fmt.Errorf("%w: %s", ErrNotEligible, look.Message)
	}

	checkout := make([]coreservice.CheckoutItemInput, 0, len(items))
	for _, it := range items {
		checkout = append(checkout, coreservice.CheckoutItemInput{ProductID: it.ProductID, Quantity: it.Quantity})
	}
	prep, err := s.orders.PrepareAndCreateOrder(ctx, coreservice.CreateCheckoutInput{
		Items:  checkout,
		Origin: order.OriginShop,
	}, nil, nil)
	if err != nil {
		return ClaimResult{}, err
	}
	if err := s.settle.SetPosPaymentGratisPlugin(ctx, prep.OrderID, nil, prep.TotalCents, Key); err != nil {
		return ClaimResult{}, fmt.Errorf("settle claim order: %w", err)
	}

	redeemItems := make([]RedeemItem, 0, len(items))
	for _, it := range items {
		redeemItems = append(redeemItems, RedeemItem{SKU: it.ProductID, Quantity: it.Quantity})
	}
	if err := s.Book(ctx, prep.OrderID, look.MemberRef, look.MemberName, quantity, redeemItems); err != nil {
		return ClaimResult{}, err
	}

	look.MemberRef = ""
	return ClaimResult{OrderID: prep.OrderID, QRPayload: prep.QRPayload, Member: look}, nil
}

// Book takes the units off the member's budget at the provider and remembers
// the booking. The order id is the reference, so a retry after a timeout books
// once; a provider that never answers gets a release for the same reference,
// because the caller rolls the order back.
func (s *service) Book(ctx context.Context, orderID, memberRef, memberName string, quantity int, items []RedeemItem) error {
	if quantity <= 0 {
		return ErrNothingClaimed
	}
	if memberRef == "" {
		return fmt.Errorf("%w: no member", ErrNotEligible)
	}
	creds, err := s.credentials(ctx)
	if err != nil {
		return err
	}

	decision, err := s.provider.Redeem(ctx, creds, RedeemRequest{
		Reference: orderID,
		MemberRef: memberRef,
		Quantity:  quantity,
		Items:     items,
	})
	if err != nil {
		s.releaseAfterFailedBooking(ctx, creds, orderID)
		return err
	}
	s.rememberProgram(ctx, decision.Program)
	if !decision.ok() {
		return fmt.Errorf("%w: %s", ErrNotEligible, decision.Message)
	}

	if _, err := s.redemptions.Create(ctx, memberRef, memberName, orderID, quantity); err != nil {
		s.releaseAfterFailedBooking(ctx, creds, orderID)
		return fmt.Errorf("record redemption: %w", err)
	}
	return nil
}

// releaseAfterFailedBooking covers the case where the provider booked but could
// not say so, or the row could not be written: the order is about to be rolled
// back, so the reference must not stay charged to the member.
func (s *service) releaseAfterFailedBooking(ctx context.Context, creds Credentials, orderID string) {
	// A fresh context: the request's own may already be done, and this is the
	// one chance to give the budget back.
	voidCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	if _, err := s.provider.Void(voidCtx, creds, orderID); err != nil {
		s.logger.Warn("membership booking could not be released",
			zap.String("orderId", orderID),
			zap.Error(err),
		)
	}
}

// ValidateOrderClaim checks the till's claim against the order it is made on
// and names the units it covers. A member never gets more units than the order
// holds of the products the tenant gives away, and the claim always falls on
// the dearest of them — the same ones the till showed the guest as free.
func (s *service) ValidateOrderClaim(ctx context.Context, orderID string, quantity int) ([]RedeemItem, error) {
	if quantity <= 0 {
		return nil, ErrNothingClaimed
	}
	cfg, err := s.config(ctx)
	if err != nil {
		return nil, err
	}
	if len(cfg.FreeProductIDs) == 0 {
		return nil, fmt.Errorf("%w: no products configured", ErrProductNotFree)
	}
	free := idSet(cfg.FreeProductIDs)

	lines, err := s.orderLines.GetByOrderID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order lines: %w", err)
	}
	eligible := make([]*ent.OrderLine, 0, len(lines))
	available := 0
	for _, line := range lines {
		// Deposit and bundle component lines are not things a guest picked.
		if line.LineType == orderline.LineTypeDeposit || line.ParentLineID != nil {
			continue
		}
		if free[line.ProductID] {
			eligible = append(eligible, line)
			available += line.Quantity
		}
	}
	if available < quantity {
		return nil, fmt.Errorf("%w: order holds %d free-eligible units, claimed %d", ErrProductNotFree, available, quantity)
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].UnitPriceCents > eligible[j].UnitPriceCents
	})
	items := make([]RedeemItem, 0, len(eligible))
	left := quantity
	for _, line := range eligible {
		if left == 0 {
			break
		}
		take := min(line.Quantity, left)
		left -= take
		items = append(items, RedeemItem{SKU: line.ProductID, Name: line.Title, Quantity: take})
	}
	return items, nil
}

// Release gives the member their units back once the order is cancelled or the
// comped units have been refunded. Rows stay unreleased when the provider
// cannot be reached, so the next refund on the order tries again.
func (s *service) Release(ctx context.Context, ord *ent.Order, in plugin.RefundedOrder) error {
	rows, err := s.redemptions.GetByOrderID(ctx, ord.ID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	returned := 0
	for _, units := range in.RefundedUnits {
		returned += units
	}

	creds, err := s.credentials(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ReleasedAt != nil {
			continue
		}
		if !in.Cancelled && returned < row.Quantity {
			continue
		}
		decision, err := s.provider.Void(ctx, creds, ord.ID)
		if err != nil {
			return err
		}
		if decision.Status != StatusOK && decision.Status != StatusAlreadyVoided && decision.Status != StatusUnknown {
			return fmt.Errorf("provider refused the release: %s", decision.Message)
		}
		if err := s.redemptions.MarkReleased(ctx, row.ID, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) existingProductIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	products, err := s.products.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("validate products: %w", err)
	}
	present := make(map[string]bool, len(products))
	for _, p := range products {
		present[p.ID] = true
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if present[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func idSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}
