package artistvoucher

import (
	"context"
	"encoding/json"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/voucher"
	"backend/internal/repository"
)

type CreateParams struct {
	ID         string
	Label      string
	ValidFrom  time.Time
	ValidUntil time.Time
	Categories json.RawMessage
	Token      string
}

type VoucherRepository interface {
	Create(ctx context.Context, p CreateParams) (*ent.Voucher, error)
	GetByID(ctx context.Context, id string) (*ent.Voucher, error)
	List(ctx context.Context) ([]*ent.Voucher, error)
	SetRevoked(ctx context.Context, id string, at time.Time) (*ent.Voucher, error)
	ListActiveRevoked(ctx context.Context, now time.Time) ([]*ent.Voucher, error)
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

func (r *voucherRepo) Create(ctx context.Context, p CreateParams) (*ent.Voucher, error) {
	e, err := r.ec(ctx).Voucher.Create().
		SetID(p.ID).
		SetLabel(p.Label).
		SetValidFrom(p.ValidFrom).
		SetValidUntil(p.ValidUntil).
		SetCategories(p.Categories).
		SetToken(p.Token).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *voucherRepo) GetByID(ctx context.Context, id string) (*ent.Voucher, error) {
	e, err := r.ec(ctx).Voucher.Get(ctx, id)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *voucherRepo) List(ctx context.Context) ([]*ent.Voucher, error) {
	rows, err := r.ec(ctx).Voucher.Query().
		Order(ent.Desc(voucher.FieldCreatedAt)).
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
