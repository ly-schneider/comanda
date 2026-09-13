package artistvoucher

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/voucher"
	"backend/internal/id"
	"backend/internal/qrsign"
	"backend/internal/repository"
	coreservice "backend/internal/service"
)

const Key = "artist-voucher"

// Bounded in runes (user-facing) and bytes: a multibyte label must not pass here
// only to fail the MaxLen(80) column or the 255-byte token encoder later.
const (
	maxLabelRunes = 80
	maxLabelBytes = 80
)

var (
	ErrEmptyLabel      = errors.New("artistvoucher: label is required")
	ErrLabelTooLong    = errors.New("artistvoucher: label exceeds 80 characters")
	ErrInvalidWindow   = errors.New("artistvoucher: valid_until must be after valid_from")
	ErrNoCategories    = errors.New("artistvoucher: at least one category is required")
	ErrUnknownCategory = errors.New("artistvoucher: unknown category")
	ErrInvalidToken    = errors.New("artistvoucher: voucher does not fit the token format")
	ErrNoTenant        = errors.New("artistvoucher: no tenant in context")
	ErrNotExpired      = errors.New("artistvoucher: only expired vouchers can be deleted")
)

// IsValidation reports whether err is a client-input error the handler should
// surface as a 400 rather than a 500.
func IsValidation(err error) bool {
	switch {
	case errors.Is(err, ErrEmptyLabel),
		errors.Is(err, ErrLabelTooLong),
		errors.Is(err, ErrInvalidWindow),
		errors.Is(err, ErrNoCategories),
		errors.Is(err, ErrUnknownCategory),
		errors.Is(err, ErrInvalidToken):
		return true
	}
	return false
}

func validateLabel(label string) error {
	if utf8.RuneCountInString(label) == 0 {
		return ErrEmptyLabel
	}
	if utf8.RuneCountInString(label) > maxLabelRunes || len(label) > maxLabelBytes {
		return ErrLabelTooLong
	}
	return nil
}

type Status string

const (
	StatusScheduled Status = "scheduled"
	StatusActive    Status = "active"
	StatusExpired   Status = "expired"
	StatusRevoked   Status = "revoked"
	StatusRedeemed  Status = "redeemed"
)

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Voucher struct {
	ID         string
	Label      string
	ValidFrom  time.Time
	ValidUntil time.Time
	RevokedAt  *time.Time
	Categories []Category
	Token      string
	CreatedAt  time.Time
	Status     Status
}

type CreateInput struct {
	Label       string
	ValidFrom   time.Time
	ValidUntil  time.Time
	CategoryIDs []string
}

type Feed struct {
	RevokedIDs []string
	AsOf       time.Time
}

// CategoryLookup resolves a category id against the RLS-scoped store, so a
// category belonging to another tenant surfaces as repository.ErrNotFound.
type CategoryLookup interface {
	GetByID(ctx context.Context, id string) (*ent.Category, error)
}

// KeyProvider hands back the tenant's Ed25519 signing key (the same key that
// signs pickup tokens), so a scanned voucher verifies under the tenant's
// published public key.
type KeyProvider interface {
	EnsureTenantKey(ctx context.Context) (ed25519.PrivateKey, ed25519.PublicKey, error)
}

type ProductLookup interface {
	GetByIDs(ctx context.Context, ids []string) ([]*ent.Product, error)
	FilterBuyable(ctx context.Context, ids []string) (map[string]struct{}, error)
}

type OrderMinter interface {
	PrepareAndCreateOrder(ctx context.Context, in coreservice.CreateCheckoutInput, userID *string, attemptID *string) (*coreservice.CheckoutPreparation, error)
}

type OrderSettler interface {
	SetPosPaymentGratisPlugin(ctx context.Context, orderID string, deviceID *string, amountCents int64, pluginKey string) error
}

type StockLookup interface {
	GetCurrentStockBatch(ctx context.Context, productIDs []string) (map[string]int, error)
}

type Service interface {
	List(ctx context.Context) ([]Voucher, error)
	Get(ctx context.Context, id string) (Voucher, error)
	Create(ctx context.Context, in CreateInput) (Voucher, error)
	Revoke(ctx context.Context, id string) (Voucher, error)
	Delete(ctx context.Context, id string) error
	Revocations(ctx context.Context) (Feed, error)

	CreateBatch(ctx context.Context, in CreateBatchInput) (Batch, error)
	ListBatches(ctx context.Context) ([]Batch, error)
	GetBatch(ctx context.Context, batchID string) (BatchDetail, error)
	RevokeBatch(ctx context.Context, batchID string) (Batch, error)
	Redeem(ctx context.Context, voucherID, deviceID string, selections []ChoiceSelection) (RedeemResult, error)
}

type service struct {
	repo       VoucherRepository
	categories CategoryLookup
	products   ProductLookup
	keys       KeyProvider
	orders     OrderMinter
	settle     OrderSettler
	stock      StockLookup
}

func NewService(
	repo VoucherRepository,
	categories CategoryLookup,
	products ProductLookup,
	keys KeyProvider,
	orders OrderMinter,
	settle OrderSettler,
	stock StockLookup,
) Service {
	return &service{
		repo:       repo,
		categories: categories,
		products:   products,
		keys:       keys,
		orders:     orders,
		settle:     settle,
		stock:      stock,
	}
}

func (s *service) List(ctx context.Context) ([]Voucher, error) {
	now := time.Now()
	rows, err := s.repo.ListByKind(ctx, voucher.KindPass)
	if err != nil {
		return nil, err
	}
	out := make([]Voucher, 0, len(rows))
	for _, row := range rows {
		v, err := toVoucher(row, now)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (Voucher, error) {
	if err := validateLabel(in.Label); err != nil {
		return Voucher{}, err
	}
	if !in.ValidUntil.After(in.ValidFrom) {
		return Voucher{}, ErrInvalidWindow
	}
	if len(in.CategoryIDs) == 0 {
		return Voucher{}, ErrNoCategories
	}

	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok || tenantID == "" {
		return Voucher{}, ErrNoTenant
	}

	snapshot := make([]Category, 0, len(in.CategoryIDs))
	names := make([]string, 0, len(in.CategoryIDs))
	for _, cid := range in.CategoryIDs {
		cat, err := s.categories.GetByID(ctx, cid)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return Voucher{}, fmt.Errorf("%w: %s", ErrUnknownCategory, cid)
			}
			return Voucher{}, err
		}
		snapshot = append(snapshot, Category{ID: cat.ID, Name: cat.Name})
		names = append(names, cat.Name)
	}

	priv, _, err := s.keys.EnsureTenantKey(ctx)
	if err != nil {
		return Voucher{}, fmt.Errorf("ensure tenant key: %w", err)
	}

	vid := id.New()
	token, err := qrsign.SignVoucher(priv, qrsign.VoucherPayload{
		TenantID:   tenantID,
		VoucherID:  vid,
		ValidFrom:  in.ValidFrom.Unix(),
		ValidUntil: in.ValidUntil.Unix(),
		Label:      in.Label,
		Categories: names,
	})
	if err != nil {
		if errors.Is(err, qrsign.ErrBadPayload) {
			return Voucher{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
		return Voucher{}, fmt.Errorf("sign voucher: %w", err)
	}

	catsJSON, err := json.Marshal(snapshot)
	if err != nil {
		return Voucher{}, fmt.Errorf("marshal categories: %w", err)
	}

	row, err := s.repo.Create(ctx, CreateParams{
		ID:         vid,
		Kind:       voucher.KindPass,
		Label:      in.Label,
		ValidFrom:  in.ValidFrom,
		ValidUntil: in.ValidUntil,
		Categories: catsJSON,
		Token:      token,
	})
	if err != nil {
		return Voucher{}, err
	}
	return toVoucher(row, time.Now())
}

// getPass loads a voucher through the pass endpoints, refusing a batch slip: it
// is owned by its batch, and revoking or deleting it individually would desync
// the batch counts from what was printed.
func (s *service) getPass(ctx context.Context, id string) (*ent.Voucher, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != voucher.KindPass {
		return nil, repository.ErrNotFound
	}
	return row, nil
}

func (s *service) Get(ctx context.Context, id string) (Voucher, error) {
	row, err := s.getPass(ctx, id)
	if err != nil {
		return Voucher{}, err
	}
	return toVoucher(row, time.Now())
}

func (s *service) Revoke(ctx context.Context, id string) (Voucher, error) {
	now := time.Now()
	row, err := s.getPass(ctx, id)
	if err != nil {
		return Voucher{}, err
	}
	if row.RevokedAt != nil {
		return toVoucher(row, now)
	}
	updated, err := s.repo.SetRevoked(ctx, id, now)
	if err != nil {
		return Voucher{}, err
	}
	return toVoucher(updated, now)
}

// Only expired vouchers may be deleted: removing a still-live one would drop it
// from the revocation feed and un-block it at offline stations.
func (s *service) Delete(ctx context.Context, id string) error {
	row, err := s.getPass(ctx, id)
	if err != nil {
		return err
	}
	if time.Now().Before(row.ValidUntil) {
		return ErrNotExpired
	}
	return s.repo.Delete(ctx, id)
}

func (s *service) Revocations(ctx context.Context) (Feed, error) {
	now := time.Now()
	rows, err := s.repo.ListActiveRevoked(ctx, now)
	if err != nil {
		return Feed{}, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return Feed{RevokedIDs: ids, AsOf: now}, nil
}

func computeStatus(now time.Time, v *ent.Voucher) Status {
	if v.RedeemedAt != nil {
		return StatusRedeemed
	}
	if v.RevokedAt != nil {
		return StatusRevoked
	}
	if !now.Before(v.ValidUntil) {
		return StatusExpired
	}
	if now.Before(v.ValidFrom) {
		return StatusScheduled
	}
	return StatusActive
}

func toVoucher(row *ent.Voucher, now time.Time) (Voucher, error) {
	var cats []Category
	if len(row.Categories) > 0 {
		if err := json.Unmarshal(row.Categories, &cats); err != nil {
			return Voucher{}, fmt.Errorf("unmarshal categories: %w", err)
		}
	}
	return Voucher{
		ID:         row.ID,
		Label:      row.Label,
		ValidFrom:  row.ValidFrom,
		ValidUntil: row.ValidUntil,
		RevokedAt:  row.RevokedAt,
		Categories: cats,
		Token:      row.Token,
		CreatedAt:  row.CreatedAt,
		Status:     computeStatus(now, row),
	}, nil
}
