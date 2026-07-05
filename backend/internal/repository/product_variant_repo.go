package repository

import (
	"context"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/productvariant"
)

type ProductVariantRepository interface {
	GetByID(ctx context.Context, id string) (*ent.ProductVariant, error)
	GetByIDs(ctx context.Context, ids []string) ([]*ent.ProductVariant, error)
	GetByProductID(ctx context.Context, productID string) ([]*ent.ProductVariant, error)
	Create(ctx context.Context, productID, value string, priceAdjustmentCents int64, isActive bool, sequence int) (*ent.ProductVariant, error)
	Update(ctx context.Context, id, value string, priceAdjustmentCents int64, isActive bool, sequence int) (*ent.ProductVariant, error)
	Delete(ctx context.Context, id string) error
	IsReferenced(ctx context.Context, id string) (bool, error)
	ExistsForProduct(ctx context.Context, productID string) (bool, error)
}

type productVariantRepo struct {
	client *ent.Client
}

func NewProductVariantRepository(client *ent.Client) ProductVariantRepository {
	return &productVariantRepo{client: client}
}

func (r *productVariantRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *productVariantRepo) GetByID(ctx context.Context, id string) (*ent.ProductVariant, error) {
	e, err := r.ec(ctx).ProductVariant.Get(ctx, id)
	if err != nil {
		return nil, translateError(err)
	}
	return e, nil
}

func (r *productVariantRepo) GetByIDs(ctx context.Context, ids []string) ([]*ent.ProductVariant, error) {
	if len(ids) == 0 {
		return []*ent.ProductVariant{}, nil
	}
	rows, err := r.ec(ctx).ProductVariant.Query().
		Where(productvariant.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *productVariantRepo) GetByProductID(ctx context.Context, productID string) ([]*ent.ProductVariant, error) {
	rows, err := r.ec(ctx).ProductVariant.Query().
		Where(productvariant.ProductIDEQ(productID)).
		Order(productvariant.BySequence(), productvariant.ByValue()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *productVariantRepo) Create(ctx context.Context, productID, value string, priceAdjustmentCents int64, isActive bool, sequence int) (*ent.ProductVariant, error) {
	created, err := r.ec(ctx).ProductVariant.Create().
		SetProductID(productID).
		SetValue(value).
		SetPriceAdjustmentCents(priceAdjustmentCents).
		SetIsActive(isActive).
		SetSequence(sequence).
		Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *productVariantRepo) Update(ctx context.Context, id, value string, priceAdjustmentCents int64, isActive bool, sequence int) (*ent.ProductVariant, error) {
	updated, err := r.ec(ctx).ProductVariant.UpdateOneID(id).
		SetValue(value).
		SetPriceAdjustmentCents(priceAdjustmentCents).
		SetIsActive(isActive).
		SetSequence(sequence).
		Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return updated, nil
}

func (r *productVariantRepo) Delete(ctx context.Context, id string) error {
	return translateError(r.ec(ctx).ProductVariant.DeleteOneID(id).Exec(ctx))
}

func (r *productVariantRepo) IsReferenced(ctx context.Context, id string) (bool, error) {
	inOrders, err := r.ec(ctx).OrderLine.Query().
		Where(orderline.VariantIDEQ(id)).
		Exist(ctx)
	if err != nil {
		return false, translateError(err)
	}
	if inOrders {
		return true, nil
	}
	inLedger, err := r.ec(ctx).InventoryLedger.Query().
		Where(inventoryledger.VariantIDEQ(id)).
		Exist(ctx)
	if err != nil {
		return false, translateError(err)
	}
	return inLedger, nil
}

func (r *productVariantRepo) ExistsForProduct(ctx context.Context, productID string) (bool, error) {
	exists, err := r.ec(ctx).ProductVariant.Query().
		Where(productvariant.ProductIDEQ(productID)).
		Exist(ctx)
	if err != nil {
		return false, translateError(err)
	}
	return exists, nil
}
