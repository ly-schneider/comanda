package artistvoucher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/voucher"
	"backend/internal/id"
	"backend/internal/qrsign"
	"backend/internal/repository"
	coreservice "backend/internal/service"
)

const (
	minBatchCount    = 1
	maxBatchCount    = 500
	maxBatchProducts = 255
	maxBatchQuantity = 255
	maxChoiceGroup   = 255
)

var (
	ErrNoProducts            = errors.New("artistvoucher: at least one product is required")
	ErrTooManyProducts       = errors.New("artistvoucher: too many products")
	ErrDuplicateProduct      = errors.New("artistvoucher: product listed more than once")
	ErrUnknownProduct        = errors.New("artistvoucher: unknown product")
	ErrProductNotSimple      = errors.New("artistvoucher: only simple products can be issued")
	ErrProductHasVariants    = errors.New("artistvoucher: product has variants")
	ErrProductUnavailable    = errors.New("artistvoucher: product is not available for sale")
	ErrInvalidChoiceGroup    = errors.New("artistvoucher: choice group is out of range")
	ErrChoiceGroupTooSmall   = errors.New("artistvoucher: a choice group needs at least two products")
	ErrInvalidSelection      = errors.New("artistvoucher: selection does not match the voucher")
	ErrNothingForThisStation = errors.New("artistvoucher: this station serves nothing left on the voucher")
	ErrInvalidQuantity       = errors.New("artistvoucher: quantity out of range")
	ErrInvalidCount          = errors.New("artistvoucher: count out of range")
	ErrNotEntitlement        = errors.New("artistvoucher: voucher is not single-use")
	ErrRevoked               = errors.New("artistvoucher: voucher is revoked")
	ErrNotYetValid           = errors.New("artistvoucher: voucher is not valid yet")
	ErrExpired               = errors.New("artistvoucher: voucher has expired")
)

type Product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Quantity    int    `json:"quantity"`
	ChoiceGroup int    `json:"choiceGroup,omitempty"`
}

type BatchProduct struct {
	Product
	Stock int
}

type Batch struct {
	ID          string
	Label       string
	ValidFrom   time.Time
	ValidUntil  time.Time
	Products    []BatchProduct
	Issued      int
	Redeemed    int
	Revoked     int
	Outstanding int
	Status      Status
	CreatedAt   time.Time
}

type BatchVoucher struct {
	ID         string
	Status     Status
	RedeemedAt *time.Time
	RevokedAt  *time.Time
	Token      string
}

type BatchDetail struct {
	Batch
	Vouchers []BatchVoucher
}

type BatchProductInput struct {
	ProductID   string
	Quantity    int
	ChoiceGroup int
}

type CreateBatchInput struct {
	Label      string
	ValidFrom  time.Time
	ValidUntil time.Time
	Products   []BatchProductInput
	Count      int
}

type RedeemResult struct {
	VoucherID       string
	Label           string
	OrderID         string
	RedeemedAt      time.Time
	AlreadyRedeemed bool
	Items           []Product
	FullyRedeemed   bool
}

func (s *service) CreateBatch(ctx context.Context, in CreateBatchInput) (Batch, error) {
	if err := validateLabel(in.Label); err != nil {
		return Batch{}, err
	}
	if !in.ValidUntil.After(in.ValidFrom) {
		return Batch{}, ErrInvalidWindow
	}
	if in.Count < minBatchCount || in.Count > maxBatchCount {
		return Batch{}, fmt.Errorf("%w: %d is outside [%d,%d]", ErrInvalidCount, in.Count, minBatchCount, maxBatchCount)
	}
	if len(in.Products) == 0 {
		return Batch{}, ErrNoProducts
	}
	if len(in.Products) > maxBatchProducts {
		return Batch{}, ErrTooManyProducts
	}

	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok || tenantID == "" {
		return Batch{}, ErrNoTenant
	}

	ids := make([]string, 0, len(in.Products))
	seen := make(map[string]struct{}, len(in.Products))
	groupSizes := make(map[int]int, len(in.Products))
	for _, p := range in.Products {
		if p.Quantity < 1 || p.Quantity > maxBatchQuantity {
			return Batch{}, fmt.Errorf("%w: %d", ErrInvalidQuantity, p.Quantity)
		}
		if p.ChoiceGroup < 0 || p.ChoiceGroup > maxChoiceGroup {
			return Batch{}, fmt.Errorf("%w: %d", ErrInvalidChoiceGroup, p.ChoiceGroup)
		}
		groupSizes[p.ChoiceGroup]++
		if _, dup := seen[p.ProductID]; dup {
			return Batch{}, fmt.Errorf("%w: %s", ErrDuplicateProduct, p.ProductID)
		}
		seen[p.ProductID] = struct{}{}
		ids = append(ids, p.ProductID)
	}

	for group, n := range groupSizes {
		if group > 0 && n < 2 {
			return Batch{}, fmt.Errorf("%w: %d", ErrChoiceGroupTooSmall, group)
		}
	}

	rows, err := s.products.GetByIDs(ctx, ids)
	if err != nil {
		return Batch{}, fmt.Errorf("load products: %w", err)
	}
	byID := make(map[string]*ent.Product, len(rows))
	for _, p := range rows {
		byID[p.ID] = p
	}

	// A slip is printed long before it is redeemed, so an unsellable product has
	// to be refused here; at redemption it would fail the whole batch in the
	// field, after the slips are handed out.
	buyable, err := s.products.FilterBuyable(ctx, ids)
	if err != nil {
		return Batch{}, fmt.Errorf("filter buyable: %w", err)
	}

	snapshot := make([]Product, 0, len(in.Products))
	lines := make([]qrsign.EntitlementLine, 0, len(in.Products))
	for _, p := range in.Products {
		prod, ok := byID[p.ProductID]
		if !ok {
			return Batch{}, fmt.Errorf("%w: %s", ErrUnknownProduct, p.ProductID)
		}
		if prod.Type != product.TypeSimple {
			return Batch{}, fmt.Errorf("%w: %s", ErrProductNotSimple, prod.Name)
		}
		if len(prod.Edges.Variants) > 0 {
			return Batch{}, fmt.Errorf("%w: %s", ErrProductHasVariants, prod.Name)
		}
		if _, ok := buyable[prod.ID]; !ok {
			return Batch{}, fmt.Errorf("%w: %s", ErrProductUnavailable, prod.Name)
		}
		snapshot = append(snapshot, Product{ID: prod.ID, Name: prod.Name, Quantity: p.Quantity, ChoiceGroup: p.ChoiceGroup})
		lines = append(lines, qrsign.EntitlementLine{ProductID: prod.ID, Quantity: p.Quantity, ChoiceGroup: p.ChoiceGroup})
	}

	productsJSON, err := json.Marshal(snapshot)
	if err != nil {
		return Batch{}, fmt.Errorf("marshal products: %w", err)
	}

	priv, _, err := s.keys.EnsureTenantKey(ctx)
	if err != nil {
		return Batch{}, fmt.Errorf("ensure tenant key: %w", err)
	}

	batchID := id.New()
	params := make([]CreateParams, 0, in.Count)
	for i := 0; i < in.Count; i++ {
		vid := id.New()
		token, err := qrsign.SignEntitlement(priv, qrsign.EntitlementPayload{
			TenantID:   tenantID,
			VoucherID:  vid,
			ValidFrom:  in.ValidFrom.Unix(),
			ValidUntil: in.ValidUntil.Unix(),
			Label:      in.Label,
			Lines:      lines,
		})
		if err != nil {
			if errors.Is(err, qrsign.ErrBadPayload) {
				return Batch{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
			}
			return Batch{}, fmt.Errorf("sign entitlement: %w", err)
		}
		params = append(params, CreateParams{
			ID:         vid,
			Kind:       voucher.KindEntitlement,
			Label:      in.Label,
			ValidFrom:  in.ValidFrom,
			ValidUntil: in.ValidUntil,
			Products:   productsJSON,
			BatchID:    batchID,
			Token:      token,
		})
	}

	created, err := s.repo.CreateMany(ctx, params)
	if err != nil {
		return Batch{}, err
	}

	stock, err := s.currentStock(ctx, ids)
	if err != nil {
		return Batch{}, err
	}
	batch, err := toBatch(created, stock, time.Now())
	if err != nil {
		return Batch{}, err
	}
	return batch, nil
}

func (s *service) ListBatches(ctx context.Context) ([]Batch, error) {
	rows, err := s.repo.ListByKind(ctx, voucher.KindEntitlement)
	if err != nil {
		return nil, err
	}

	grouped, batchIDs := groupByBatch(rows)
	productIDs := make([]string, 0, len(batchIDs))
	seen := make(map[string]struct{})
	for _, batchRows := range grouped {
		snapshot, err := decodeProducts(batchRows[0])
		if err != nil {
			return nil, err
		}
		for _, p := range snapshot {
			if _, ok := seen[p.ID]; ok {
				continue
			}
			seen[p.ID] = struct{}{}
			productIDs = append(productIDs, p.ID)
		}
	}
	stock, err := s.currentStock(ctx, productIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	out := make([]Batch, 0, len(batchIDs))
	for _, batchID := range batchIDs {
		batch, err := toBatch(grouped[batchID], stock, now)
		if err != nil {
			return nil, err
		}
		out = append(out, batch)
	}
	return out, nil
}

func (s *service) GetBatch(ctx context.Context, batchID string) (BatchDetail, error) {
	rows, err := s.repo.ListByBatch(ctx, batchID)
	if err != nil {
		return BatchDetail{}, err
	}
	if len(rows) == 0 {
		return BatchDetail{}, repository.ErrNotFound
	}

	snapshot, err := decodeProducts(rows[0])
	if err != nil {
		return BatchDetail{}, err
	}
	ids := make([]string, 0, len(snapshot))
	for _, p := range snapshot {
		ids = append(ids, p.ID)
	}
	stock, err := s.currentStock(ctx, ids)
	if err != nil {
		return BatchDetail{}, err
	}

	now := time.Now()
	batch, err := toBatch(rows, stock, now)
	if err != nil {
		return BatchDetail{}, err
	}
	vouchers := make([]BatchVoucher, 0, len(rows))
	for _, row := range rows {
		vouchers = append(vouchers, BatchVoucher{
			ID:         row.ID,
			Status:     computeStatus(now, row),
			RedeemedAt: row.RedeemedAt,
			RevokedAt:  row.RevokedAt,
			Token:      row.Token,
		})
	}
	return BatchDetail{Batch: batch, Vouchers: vouchers}, nil
}

func (s *service) RevokeBatch(ctx context.Context, batchID string) (Batch, error) {
	rows, err := s.repo.ListByBatch(ctx, batchID)
	if err != nil {
		return Batch{}, err
	}
	if len(rows) == 0 {
		return Batch{}, repository.ErrNotFound
	}
	if _, err := s.repo.RevokeBatchOutstanding(ctx, batchID, time.Now()); err != nil {
		return Batch{}, err
	}
	detail, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return Batch{}, err
	}
	return detail.Batch, nil
}

type ChoiceSelection struct {
	ChoiceGroup int
	ProductID   string
}

type ChoiceGroupView struct {
	ChoiceGroup int
	Options     []Product
}

type ChoiceRequiredError struct {
	Groups []ChoiceGroupView
}

func (e *ChoiceRequiredError) Error() string {
	return fmt.Sprintf("artistvoucher: %d choice group(s) need a selection", len(e.Groups))
}

func EntitlementLines(products []Product) []string {
	groups := make(map[int][]Product)
	order := make([]int, 0, len(products))
	out := make([]string, 0, len(products))

	for _, p := range products {
		if p.ChoiceGroup <= 0 {
			out = append(out, fmt.Sprintf("%d× %s", p.Quantity, p.Name))
			continue
		}
		if _, seen := groups[p.ChoiceGroup]; !seen {
			order = append(order, p.ChoiceGroup)
		}
		groups[p.ChoiceGroup] = append(groups[p.ChoiceGroup], p)
	}
	sort.Ints(order)

	for _, g := range order {
		parts := make([]string, 0, len(groups[g]))
		for _, o := range groups[g] {
			parts = append(parts, fmt.Sprintf("%d× %s", o.Quantity, o.Name))
		}
		out = append(out, strings.Join(parts, " oder "))
	}
	return out
}

type Position struct {
	Key      string
	Products []Product
}

type RedeemedPosition struct {
	Key        string    `json:"key"`
	ProductID  string    `json:"productId"`
	Name       string    `json:"name"`
	Quantity   int       `json:"quantity"`
	OrderID    string    `json:"orderId"`
	DeviceID   string    `json:"deviceId"`
	RedeemedAt time.Time `json:"redeemedAt"`
}

func positionsOf(products []Product) []Position {
	groups := make(map[int][]Product)
	order := make([]int, 0, len(products))
	out := make([]Position, 0, len(products))

	for _, p := range products {
		if p.ChoiceGroup <= 0 {
			out = append(out, Position{Key: "p:" + p.ID, Products: []Product{p}})
			continue
		}
		if _, seen := groups[p.ChoiceGroup]; !seen {
			order = append(order, p.ChoiceGroup)
		}
		groups[p.ChoiceGroup] = append(groups[p.ChoiceGroup], p)
	}
	sort.Ints(order)
	for _, g := range order {
		out = append(out, Position{Key: fmt.Sprintf("g:%d", g), Products: groups[g]})
	}
	return out
}

func (p Position) choiceGroup() int {
	var g int
	if _, err := fmt.Sscanf(p.Key, "g:%d", &g); err == nil {
		return g
	}
	return 0
}

func (p Position) servableBy(serves map[string]struct{}) []Product {
	out := make([]Product, 0, len(p.Products))
	for _, prod := range p.Products {
		if _, ok := serves[prod.ID]; ok {
			out = append(out, prod)
		}
	}
	return out
}

func decodeRedeemedPositions(row *ent.Voucher) ([]RedeemedPosition, error) {
	if len(row.RedeemedPositions) == 0 {
		return nil, nil
	}
	var out []RedeemedPosition
	if err := json.Unmarshal(row.RedeemedPositions, &out); err != nil {
		return nil, fmt.Errorf("unmarshal redeemed positions: %w", err)
	}
	return out, nil
}

func (s *service) stationProducts(ctx context.Context, deviceID string) (map[string]struct{}, error) {
	if deviceID == "" {
		return nil, ErrNothingForThisStation
	}
	ids, err := s.stations.ListStationProductIDs(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list station products: %w", err)
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

func (s *service) Redeem(ctx context.Context, voucherID, deviceID string, selections []ChoiceSelection) (RedeemResult, error) {
	if err := s.repo.LockByID(ctx, voucherID); err != nil {
		return RedeemResult{}, err
	}
	row, err := s.repo.GetByID(ctx, voucherID)
	if err != nil {
		return RedeemResult{}, err
	}
	if row.Kind != voucher.KindEntitlement {
		return RedeemResult{}, ErrNotEntitlement
	}
	products, err := decodeProducts(row)
	if err != nil {
		return RedeemResult{}, err
	}
	if len(products) == 0 {
		return RedeemResult{}, ErrNoProducts
	}
	done, err := decodeRedeemedPositions(row)
	if err != nil {
		return RedeemResult{}, err
	}

	positions := positionsOf(products)
	spent := make(map[string]RedeemedPosition, len(done))
	for _, d := range done {
		spent[d.Key] = d
	}

	serves, err := s.stationProducts(ctx, deviceID)
	if err != nil {
		return RedeemResult{}, err
	}

	if prev := priorRedemption(positions, spent, serves, deviceID); prev != nil {
		return *prev, nil
	}

	if row.RevokedAt != nil {
		return RedeemResult{}, ErrRevoked
	}
	now := time.Now()
	if now.Before(row.ValidFrom) {
		return RedeemResult{}, ErrNotYetValid
	}
	if !now.Before(row.ValidUntil) {
		return RedeemResult{}, ErrExpired
	}

	picked := make(map[int]string, len(selections))
	for _, sel := range selections {
		picked[sel.ChoiceGroup] = sel.ProductID
	}

	var (
		take    []Product
		keys    []string
		pending []ChoiceGroupView
	)
	for _, pos := range positions {
		if _, gone := spent[pos.Key]; gone {
			continue
		}
		options := pos.servableBy(serves)
		if len(options) == 0 {
			continue
		}
		if len(options) == 1 {
			take = append(take, options[0])
			keys = append(keys, pos.Key)
			continue
		}
		group := pos.choiceGroup()
		choice, ok := picked[group]
		if !ok {
			pending = append(pending, ChoiceGroupView{ChoiceGroup: group, Options: options})
			continue
		}
		idx := -1
		for i, o := range options {
			if o.ID == choice {
				idx = i
				break
			}
		}
		if idx < 0 {
			return RedeemResult{}, ErrInvalidSelection
		}
		take = append(take, options[idx])
		keys = append(keys, pos.Key)
	}

	if len(pending) > 0 {
		return RedeemResult{}, &ChoiceRequiredError{Groups: pending}
	}
	if len(take) == 0 {
		return RedeemResult{}, ErrNothingForThisStation
	}

	checkout := make([]coreservice.CheckoutItemInput, 0, len(take))
	for _, it := range take {
		checkout = append(checkout, coreservice.CheckoutItemInput{ProductID: it.ID, Quantity: it.Quantity})
	}
	prep, err := s.orders.PrepareAndCreateOrder(ctx, coreservice.CreateCheckoutInput{
		Items:  checkout,
		Origin: order.OriginPos,
	}, nil, nil)
	if err != nil {
		return RedeemResult{}, err
	}

	var devicePtr *string
	if deviceID != "" {
		devicePtr = &deviceID
	}
	if err := s.settle.SetPosPaymentGratisPlugin(ctx, prep.OrderID, devicePtr, prep.TotalCents, Key); err != nil {
		return RedeemResult{}, fmt.Errorf("settle voucher order: %w", err)
	}

	for i, it := range take {
		done = append(done, RedeemedPosition{
			Key:        keys[i],
			ProductID:  it.ID,
			Name:       it.Name,
			Quantity:   it.Quantity,
			OrderID:    prep.OrderID,
			DeviceID:   deviceID,
			RedeemedAt: now,
		})
	}
	logJSON, err := json.Marshal(done)
	if err != nil {
		return RedeemResult{}, fmt.Errorf("marshal redeemed positions: %w", err)
	}

	fully := len(done) >= len(positions)
	if _, err := s.repo.SetPartiallyRedeemed(ctx, row.ID, logJSON, fully, RedeemParams{
		At: now, OrderID: prep.OrderID, DeviceID: deviceID,
	}); err != nil {
		return RedeemResult{}, err
	}

	return RedeemResult{
		VoucherID:     row.ID,
		Label:         row.Label,
		OrderID:       prep.OrderID,
		RedeemedAt:    now,
		Items:         take,
		FullyRedeemed: fully,
	}, nil
}

func priorRedemption(positions []Position, spent map[string]RedeemedPosition, serves map[string]struct{}, deviceID string) *RedeemResult {
	if deviceID == "" {
		return nil
	}
	var (
		items   []Product
		orderID string
		at      time.Time
	)
	for _, pos := range positions {
		if len(pos.servableBy(serves)) == 0 {
			continue
		}
		d, gone := spent[pos.Key]
		if !gone {
			return nil
		}
		if d.DeviceID != deviceID {
			continue
		}
		items = append(items, Product{ID: d.ProductID, Name: d.Name, Quantity: d.Quantity})
		orderID = d.OrderID
		at = d.RedeemedAt
	}
	if len(items) == 0 {
		return nil
	}
	return &RedeemResult{
		OrderID:         orderID,
		RedeemedAt:      at,
		AlreadyRedeemed: true,
		Items:           items,
	}
}

func (s *service) currentStock(ctx context.Context, productIDs []string) (map[string]int, error) {
	if len(productIDs) == 0 {
		return map[string]int{}, nil
	}
	stock, err := s.stock.GetCurrentStockBatch(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("load stock: %w", err)
	}
	return stock, nil
}

func decodeProducts(row *ent.Voucher) ([]Product, error) {
	if len(row.Products) == 0 {
		return nil, nil
	}
	var out []Product
	if err := json.Unmarshal(row.Products, &out); err != nil {
		return nil, fmt.Errorf("unmarshal products: %w", err)
	}
	return out, nil
}

func groupByBatch(rows []*ent.Voucher) (map[string][]*ent.Voucher, []string) {
	grouped := make(map[string][]*ent.Voucher)
	batchIDs := make([]string, 0)
	for _, row := range rows {
		if row.BatchID == "" {
			continue
		}
		if _, ok := grouped[row.BatchID]; !ok {
			batchIDs = append(batchIDs, row.BatchID)
		}
		grouped[row.BatchID] = append(grouped[row.BatchID], row)
	}
	return grouped, batchIDs
}

func toBatch(rows []*ent.Voucher, stock map[string]int, now time.Time) (Batch, error) {
	if len(rows) == 0 {
		return Batch{}, repository.ErrNotFound
	}
	head := rows[0]
	snapshot, err := decodeProducts(head)
	if err != nil {
		return Batch{}, err
	}
	products := make([]BatchProduct, 0, len(snapshot))
	for _, p := range snapshot {
		products = append(products, BatchProduct{Product: p, Stock: stock[p.ID]})
	}

	batch := Batch{
		ID:         head.BatchID,
		Label:      head.Label,
		ValidFrom:  head.ValidFrom,
		ValidUntil: head.ValidUntil,
		Products:   products,
		Issued:     len(rows),
		CreatedAt:  head.CreatedAt,
	}
	for _, row := range rows {
		switch {
		case row.RedeemedAt != nil:
			batch.Redeemed++
		case row.RevokedAt != nil:
			batch.Revoked++
		}
		if row.CreatedAt.Before(batch.CreatedAt) {
			batch.CreatedAt = row.CreatedAt
		}
	}
	batch.Outstanding = batch.Issued - batch.Redeemed - batch.Revoked
	batch.Status = batchStatus(now, head, batch)
	return batch, nil
}

func batchStatus(now time.Time, head *ent.Voucher, b Batch) Status {
	// Nothing left in the field ends the batch whether the slips were redeemed
	// or revoked; only a batch with outstanding slips defers to the window.
	if b.Outstanding == 0 {
		if b.Revoked > 0 {
			return StatusRevoked
		}
		return StatusExpired
	}
	if !now.Before(head.ValidUntil) {
		return StatusExpired
	}
	if now.Before(head.ValidFrom) {
		return StatusScheduled
	}
	return StatusActive
}
