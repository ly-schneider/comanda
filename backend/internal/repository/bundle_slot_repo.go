package repository

import (
	"context"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/bundleslot"
)

type BundleSlotRepository interface {
	Create(ctx context.Context, bundleProductID string, name string, sequence int) (*ent.BundleSlot, error)
	GetByID(ctx context.Context, id string) (*ent.BundleSlot, error)
	GetByBundleProductID(ctx context.Context, bundleProductID string) ([]*ent.BundleSlot, error)
	GetByBundleProductIDs(ctx context.Context, bundleProductIDs []string) ([]*ent.BundleSlot, error)
	Update(ctx context.Context, id, bundleProductID string, name string, sequence int) (*ent.BundleSlot, error)
	Delete(ctx context.Context, id string) error
	DeleteByBundleProductID(ctx context.Context, bundleProductID string) error
}

type bundleSlotRepo struct {
	client *ent.Client
}

func NewBundleSlotRepository(client *ent.Client) BundleSlotRepository {
	return &bundleSlotRepo{client: client}
}

func (r *bundleSlotRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *bundleSlotRepo) Create(ctx context.Context, bundleProductID string, name string, sequence int) (*ent.BundleSlot, error) {
	created, err := r.ec(ctx).BundleSlot.Create().
		SetBundleProductID(bundleProductID).
		SetName(name).
		SetSequence(sequence).
		Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *bundleSlotRepo) GetByID(ctx context.Context, id string) (*ent.BundleSlot, error) {
	e, err := r.ec(ctx).BundleSlot.Query().
		Where(bundleslot.ID(id)).
		WithOptions(func(oq *ent.BundleSlotOptionQuery) {
			oq.WithOptionProduct()
		}).
		Only(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return e, nil
}

func (r *bundleSlotRepo) GetByBundleProductID(ctx context.Context, bundleProductID string) ([]*ent.BundleSlot, error) {
	rows, err := r.ec(ctx).BundleSlot.Query().
		Where(bundleslot.BundleProductIDEQ(bundleProductID)).
		WithOptions(func(oq *ent.BundleSlotOptionQuery) {
			oq.WithOptionProduct()
		}).
		Order(bundleslot.BySequence()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *bundleSlotRepo) GetByBundleProductIDs(ctx context.Context, bundleProductIDs []string) ([]*ent.BundleSlot, error) {
	if len(bundleProductIDs) == 0 {
		return nil, nil
	}
	rows, err := r.ec(ctx).BundleSlot.Query().
		Where(bundleslot.BundleProductIDIn(bundleProductIDs...)).
		WithOptions(func(oq *ent.BundleSlotOptionQuery) {
			oq.WithOptionProduct()
		}).
		Order(bundleslot.BySequence()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *bundleSlotRepo) Update(ctx context.Context, id, bundleProductID string, name string, sequence int) (*ent.BundleSlot, error) {
	updated, err := r.ec(ctx).BundleSlot.UpdateOneID(id).
		SetName(name).
		SetSequence(sequence).
		SetBundleProductID(bundleProductID).
		Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return updated, nil
}

func (r *bundleSlotRepo) Delete(ctx context.Context, id string) error {
	return translateError(r.ec(ctx).BundleSlot.DeleteOneID(id).Exec(ctx))
}

func (r *bundleSlotRepo) DeleteByBundleProductID(ctx context.Context, bundleProductID string) error {
	_, err := r.ec(ctx).BundleSlot.Delete().
		Where(bundleslot.BundleProductIDEQ(bundleProductID)).
		Exec(ctx)
	return translateError(err)
}
