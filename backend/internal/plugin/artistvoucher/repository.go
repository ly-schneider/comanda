package artistvoucher

import (
	"context"
	"encoding/json"
	"time"

	"entgo.io/ent/dialect/sql"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/voucher"
	"backend/internal/repository"
)

type CreateParams struct {
	ID         string
	Kind       voucher.Kind
	Label      string
	ValidFrom  time.Time
	ValidUntil time.Time
	Categories json.RawMessage
	Products   json.RawMessage
	BatchID    string
	Token      string
}

type RedeemParams struct {
	At       time.Time
	OrderID  string
	DeviceID string
}

type VoucherRepository interface {
	Create(ctx context.Context, p CreateParams) (*ent.Voucher, error)
	CreateMany(ctx context.Context, params []CreateParams) ([]*ent.Voucher, error)
	GetByID(ctx context.Context, id string) (*ent.Voucher, error)
	LockByID(ctx context.Context, id string) error
	ListByKind(ctx context.Context, kind voucher.Kind) ([]*ent.Voucher, error)
	ListByBatch(ctx context.Context, batchID string) ([]*ent.Voucher, error)
	SetRevoked(ctx context.Context, id string, at time.Time) (*ent.Voucher, error)
	SetRedeemed(ctx context.Context, id string, p RedeemParams) (*ent.Voucher, error)
	RevokeBatchOutstanding(ctx context.Context, batchID string, at time.Time) (int, error)
	ListActiveRevoked(ctx context.Context, now time.Time) ([]*ent.Voucher, error)
	Delete(ctx context.Context, id string) error
}

type voucherRepo struct {
	client *ent.Client
}

func NewVoucherRepository(client *ent.Client) VoucherRepository {
	return &voucherRepo{client: client}
}

func (r *voucherRepo) ec(ctx context.Context) *ent.Client {
	return repository.ClientFromContext(ctx, r.client)
}

func (r *voucherRepo) builder(ctx context.Context, p CreateParams) *ent.VoucherCreate {
	b := r.ec(ctx).Voucher.Create().
		SetID(p.ID).
		SetLabel(p.Label).
		SetValidFrom(p.ValidFrom).
		SetValidUntil(p.ValidUntil).
		SetToken(p.Token)
	if p.Kind != "" {
		b.SetKind(p.Kind)
	}
	if len(p.Categories) > 0 {
		b.SetCategories(p.Categories)
	}
	if len(p.Products) > 0 {
		b.SetProducts(p.Products)
	}
	if p.BatchID != "" {
		b.SetBatchID(p.BatchID)
	}
	return b
}

func (r *voucherRepo) Create(ctx context.Context, p CreateParams) (*ent.Voucher, error) {
	e, err := r.builder(ctx, p).Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *voucherRepo) CreateMany(ctx context.Context, params []CreateParams) ([]*ent.Voucher, error) {
	builders := make([]*ent.VoucherCreate, 0, len(params))
	for _, p := range params {
		builders = append(builders, r.builder(ctx, p))
	}
	rows, err := r.ec(ctx).Voucher.CreateBulk(builders...).Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}

func (r *voucherRepo) GetByID(ctx context.Context, id string) (*ent.Voucher, error) {
	e, err := r.ec(ctx).Voucher.Get(ctx, id)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

// LockByID takes a FOR UPDATE row lock on the voucher within the request tx, so
// two concurrent redemptions of the same slip serialize: the second blocks here,
// then sees redeemed_at already set and returns the original order instead of
// minting a second one.
func (r *voucherRepo) LockByID(ctx context.Context, id string) error {
	var rows []struct {
		ID string `json:"id"`
	}
	err := r.ec(ctx).Voucher.Query().
		Where(voucher.ID(id)).
		Modify(func(s *sql.Selector) {
			s.Select(s.C(voucher.FieldID)).ForUpdate()
		}).
		Scan(ctx, &rows)
	if err != nil {
		return repository.TranslateError(err)
	}
	if len(rows) == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *voucherRepo) ListByKind(ctx context.Context, kind voucher.Kind) ([]*ent.Voucher, error) {
	rows, err := r.ec(ctx).Voucher.Query().
		Where(voucher.KindEQ(kind)).
		Order(ent.Desc(voucher.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}

func (r *voucherRepo) ListByBatch(ctx context.Context, batchID string) ([]*ent.Voucher, error) {
	rows, err := r.ec(ctx).Voucher.Query().
		Where(voucher.BatchIDEQ(batchID)).
		Order(ent.Asc(voucher.FieldCreatedAt), ent.Asc(voucher.FieldID)).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}

func (r *voucherRepo) SetRevoked(ctx context.Context, id string, at time.Time) (*ent.Voucher, error) {
	e, err := r.ec(ctx).Voucher.UpdateOneID(id).
		SetRevokedAt(at).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *voucherRepo) SetRedeemed(ctx context.Context, id string, p RedeemParams) (*ent.Voucher, error) {
	b := r.ec(ctx).Voucher.UpdateOneID(id).
		SetRedeemedAt(p.At).
		SetRedeemedOrderID(p.OrderID)
	if p.DeviceID != "" {
		b.SetRedeemedDeviceID(p.DeviceID)
	}
	e, err := b.Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *voucherRepo) RevokeBatchOutstanding(ctx context.Context, batchID string, at time.Time) (int, error) {
	n, err := r.ec(ctx).Voucher.Update().
		Where(
			voucher.BatchIDEQ(batchID),
			voucher.RevokedAtIsNil(),
			voucher.RedeemedAtIsNil(),
		).
		SetRevokedAt(at).
		Save(ctx)
	if err != nil {
		return 0, repository.TranslateError(err)
	}
	return n, nil
}

func (r *voucherRepo) Delete(ctx context.Context, id string) error {
	return repository.TranslateError(r.ec(ctx).Voucher.DeleteOneID(id).Exec(ctx))
}

func (r *voucherRepo) ListActiveRevoked(ctx context.Context, now time.Time) ([]*ent.Voucher, error) {
	rows, err := r.ec(ctx).Voucher.Query().
		Where(
			voucher.RevokedAtNotNil(),
			voucher.ValidUntilGT(now),
		).
		Order(ent.Desc(voucher.FieldRevokedAt)).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}
