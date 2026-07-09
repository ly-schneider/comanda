package repository

import (
	"context"

	"backend/internal/generated/ent"
)

type BundleSlotOptionRepository interface {
	Create(ctx context.Context, bundleSlotID, optionProductID string) (*ent.BundleSlotOption, error)
	CreateBatch(ctx context.Context, bundleSlotID string, optionProductIDs []string) ([]*ent.BundleSlotOption, error)
	GetByBundleSlotID(ctx context.Context, bundleSlotID string) ([]*ent.BundleSlotOption, error)
	Delete(ctx context.Context, bundleSlotID, optionProductID string) error
	DeleteByBundleSlotID(ctx context.Context, bundleSlotID string) error
	CountByOptionProductID(ctx context.Context, optionProductID string) (int64, error)
}

type bundleSlotOptionRepo struct {
	client *ent.Client
}

func NewBundleSlotOptionRepository(client *ent.Client) BundleSlotOptionRepository {
	return &bundleSlotOptionRepo{client: client}
}

func (r *bundleSlotOptionRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *bundleSlotOptionRepo) Create(ctx context.Context, bundleSlotID, optionProductID string) (*ent.BundleSlotOption, error) {
	created, err := r.ec(ctx).BundleSlotOption.Create().
		SetBundleSlotID(bundleSlotID).
		SetOptionProductID(optionProductID).
		Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *bundleSlotOptionRepo) CreateBatch(ctx context.Context, bundleSlotID string, optionProductIDs []string) ([]*ent.BundleSlotOption, error) {
	if len(optionProductIDs) == 0 {
		return nil, nil
	}
	builders := make([]*ent.BundleSlotOptionCreate, len(optionProductIDs))
	for i, pid := range optionProductIDs {
		builders[i] = r.ec(ctx).BundleSlotOption.Create().
			SetBundleSlotID(bundleSlotID).
			SetOptionProductID(pid)
	}
	created, err := r.ec(ctx).BundleSlotOption.CreateBulk(builders...).Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *bundleSlotOptionRepo) GetByBundleSlotID(ctx context.Context, bundleSlotID string) ([]*ent.BundleSlotOption, error) {
	rows, err := r.ec(ctx).BundleSlotOption.Query().
		Where(entBundleSlotOptionBundleSlotID(bundleSlotID)).
		WithOptionProduct().
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *bundleSlotOptionRepo) Delete(ctx context.Context, bundleSlotID, optionProductID string) error {
	_, err := r.ec(ctx).BundleSlotOption.Delete().
		Where(
			entBundleSlotOptionBundleSlotID(bundleSlotID),
			entBundleSlotOptionProductID(optionProductID),
		).
		Exec(ctx)
	return translateError(err)
}

func (r *bundleSlotOptionRepo) DeleteByBundleSlotID(ctx context.Context, bundleSlotID string) error {
	_, err := r.ec(ctx).BundleSlotOption.Delete().
		Where(entBundleSlotOptionBundleSlotID(bundleSlotID)).
		Exec(ctx)
	return translateError(err)
}

func (r *bundleSlotOptionRepo) CountByOptionProductID(ctx context.Context, optionProductID string) (int64, error) {
	count, err := r.ec(ctx).BundleSlotOption.Query().
		Where(entBundleSlotOptionProductID(optionProductID)).
		Count(ctx)
	if err != nil {
		return 0, translateError(err)
	}
	return int64(count), nil
}
