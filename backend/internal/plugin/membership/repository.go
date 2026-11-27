package membership

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/membershipredemption"
	"backend/internal/repository"
)

type RedemptionRepository interface {
	Create(ctx context.Context, memberRef, memberName, orderID string, quantity int) (*ent.MembershipRedemption, error)
	GetByOrderID(ctx context.Context, orderID string) ([]*ent.MembershipRedemption, error)
	MarkReleased(ctx context.Context, id string, at time.Time) error
}

type redemptionRepo struct {
	client *ent.Client
}

func NewRedemptionRepository(client *ent.Client) RedemptionRepository {
	return &redemptionRepo{client: client}
}

func (r *redemptionRepo) ec(ctx context.Context) *ent.Client {
	return repository.ClientFromContext(ctx, r.client)
}

func (r *redemptionRepo) Create(ctx context.Context, memberRef, memberName, orderID string, quantity int) (*ent.MembershipRedemption, error) {
	e, err := r.ec(ctx).MembershipRedemption.Create().
		SetMemberRef(memberRef).
		SetMemberName(memberName).
		SetOrderID(orderID).
		SetQuantity(quantity).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *redemptionRepo) GetByOrderID(ctx context.Context, orderID string) ([]*ent.MembershipRedemption, error) {
	rows, err := r.ec(ctx).MembershipRedemption.Query().
		Where(membershipredemption.OrderIDEQ(orderID)).
		Order(membershipredemption.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}

func (r *redemptionRepo) MarkReleased(ctx context.Context, id string, at time.Time) error {
	if err := r.ec(ctx).MembershipRedemption.UpdateOneID(id).
		SetReleasedAt(at).
		Exec(ctx); err != nil {
		return repository.TranslateError(err)
	}
	return nil
}
