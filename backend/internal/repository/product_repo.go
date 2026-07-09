package repository

import (
	"context"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/productvariant"

	"entgo.io/ent/dialect/sql"
)

func variantsOrdered(q *ent.ProductVariantQuery) {
	q.Order(productvariant.BySequence(), productvariant.ByValue())
}

type ProductRepository struct {
	client *ent.Client
}

func NewProductRepository(client *ent.Client) *ProductRepository {
	return &ProductRepository{client: client}
}

func (r *ProductRepository) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *ProductRepository) Create(ctx context.Context, categoryID string, productType product.Type, name string, priceCents, depositCents int64, isActive bool, image *string, description *string, jetonID *string, variantLabel *string) (*ent.Product, error) {
	builder := r.ec(ctx).Product.Create().
		SetCategoryID(categoryID).
		SetType(productType).
		SetName(name).
		SetPriceCents(priceCents).
		SetDepositCents(depositCents).
		SetIsActive(isActive)
	if image != nil {
		builder.SetImage(*image)
	}
	if description != nil {
		builder.SetDescription(*description)
	}
	if jetonID != nil {
		builder.SetJetonID(*jetonID)
	}
	if variantLabel != nil {
		builder.SetVariantLabel(*variantLabel)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *ProductRepository) GetByID(ctx context.Context, id string) (*ent.Product, error) {
	e, err := r.ec(ctx).Product.Get(ctx, id)
	if err != nil {
		return nil, translateError(err)
	}
	return e, nil
}

func (r *ProductRepository) GetByIDWithRelations(ctx context.Context, id string) (*ent.Product, error) {
	e, err := r.ec(ctx).Product.Query().
		Where(product.ID(id)).
		WithCategory().
		WithJeton().
		WithVariants(variantsOrdered).
		WithBundleSlots(func(q *ent.BundleSlotQuery) {
			q.WithOptions(func(oq *ent.BundleSlotOptionQuery) {
				oq.WithOptionProduct(func(pq *ent.ProductQuery) {
					pq.WithJeton()
				})
			})
		}).
		Only(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return e, nil
}

func (r *ProductRepository) GetAll(ctx context.Context) ([]*ent.Product, error) {
	rows, err := r.ec(ctx).Product.Query().
		WithCategory().
		WithJeton().
		WithVariants(variantsOrdered).
		WithBundleSlots(func(q *ent.BundleSlotQuery) {
			q.WithOptions(func(oq *ent.BundleSlotOptionQuery) {
				oq.WithOptionProduct(func(pq *ent.ProductQuery) {
					pq.WithJeton()
				})
			})
		}).
		Order(product.ByName()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *ProductRepository) GetAllActive(ctx context.Context) ([]*ent.Product, error) {
	rows, err := r.ec(ctx).Product.Query().
		Where(product.IsActive(true)).
		WithCategory().
		WithJeton().
		WithVariants(variantsOrdered).
		WithBundleSlots(func(q *ent.BundleSlotQuery) {
			q.WithOptions(func(oq *ent.BundleSlotOptionQuery) {
				oq.WithOptionProduct(func(pq *ent.ProductQuery) {
					pq.WithJeton()
				})
			})
		}).
		Order(product.ByName()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *ProductRepository) GetByCategory(ctx context.Context, categoryID string) ([]*ent.Product, error) {
	rows, err := r.ec(ctx).Product.Query().
		Where(product.CategoryIDEQ(categoryID)).
		WithCategory().
		WithJeton().
		WithVariants(variantsOrdered).
		WithBundleSlots(func(q *ent.BundleSlotQuery) {
			q.WithOptions(func(oq *ent.BundleSlotOptionQuery) {
				oq.WithOptionProduct(func(pq *ent.ProductQuery) {
					pq.WithJeton()
				})
			})
		}).
		Order(product.ByName()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *ProductRepository) GetByCategoryActive(ctx context.Context, categoryID string) ([]*ent.Product, error) {
	rows, err := r.ec(ctx).Product.Query().
		Where(
			product.CategoryIDEQ(categoryID),
			product.IsActive(true),
		).
		WithCategory().
		WithJeton().
		WithVariants(variantsOrdered).
		WithBundleSlots(func(q *ent.BundleSlotQuery) {
			q.WithOptions(func(oq *ent.BundleSlotOptionQuery) {
				oq.WithOptionProduct(func(pq *ent.ProductQuery) {
					pq.WithJeton()
				})
			})
		}).
		Order(product.ByName()).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *ProductRepository) GetByIDs(ctx context.Context, ids []string) ([]*ent.Product, error) {
	rows, err := r.ec(ctx).Product.Query().
		Where(product.IDIn(ids...)).
		WithCategory().
		WithJeton().
		WithVariants(variantsOrdered).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *ProductRepository) Update(ctx context.Context, id, categoryID string, productType product.Type, name string, priceCents, depositCents int64, isActive bool, image *string, description *string, jetonID *string, variantLabel *string) (*ent.Product, error) {
	builder := r.ec(ctx).Product.UpdateOneID(id).
		SetCategoryID(categoryID).
		SetType(productType).
		SetName(name).
		SetPriceCents(priceCents).
		SetDepositCents(depositCents).
		SetIsActive(isActive)
	if image != nil {
		builder.SetImage(*image)
	} else {
		builder.ClearImage()
	}
	if description != nil {
		builder.SetDescription(*description)
	} else {
		builder.ClearDescription()
	}
	if jetonID != nil {
		builder.SetJetonID(*jetonID)
	} else {
		builder.ClearJetonID()
	}
	if variantLabel != nil {
		builder.SetVariantLabel(*variantLabel)
	} else {
		builder.ClearVariantLabel()
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return updated, nil
}

func (r *ProductRepository) Delete(ctx context.Context, id string) error {
	return translateError(r.ec(ctx).Product.DeleteOneID(id).Exec(ctx))
}

func (r *ProductRepository) GetBundles(ctx context.Context, q *string, active *bool, limit, offset int) ([]*ent.Product, int64, error) {
	// Build count query
	countQ := r.ec(ctx).Product.Query().
		Where(product.TypeEQ(product.TypeBundle))
	if active != nil {
		countQ = countQ.Where(product.IsActive(*active))
	}
	if q != nil && *q != "" {
		countQ = countQ.Where(product.NameContainsFold(*q))
	}
	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, translateError(err)
	}

	// Build data query
	dataQ := r.ec(ctx).Product.Query().
		Where(product.TypeEQ(product.TypeBundle)).
		WithCategory().
		WithJeton().
		WithVariants(variantsOrdered).
		WithBundleSlots(func(msq *ent.BundleSlotQuery) {
			msq.WithOptions(func(oq *ent.BundleSlotOptionQuery) {
				oq.WithOptionProduct(func(pq *ent.ProductQuery) {
					pq.WithJeton()
				})
			})
		})
	if active != nil {
		dataQ = dataQ.Where(product.IsActive(*active))
	}
	if q != nil && *q != "" {
		dataQ = dataQ.Where(product.NameContainsFold(*q))
	}
	rows, err := dataQ.
		Order(product.ByName()).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, translateError(err)
	}

	return rows, int64(total), nil
}

func (r *ProductRepository) CountActiveWithoutJeton(ctx context.Context) (int64, error) {
	count, err := r.ec(ctx).Product.Query().
		Where(
			product.IsActive(true),
			product.TypeEQ(product.TypeSimple),
			product.JetonIDIsNil(),
		).
		Count(ctx)
	if err != nil {
		return 0, translateError(err)
	}
	return int64(count), nil
}

func (r *ProductRepository) CountByJetonIDs(ctx context.Context, ids []string) (map[string]int64, error) {
	result := make(map[string]int64)
	if len(ids) == 0 {
		return result, nil
	}

	// Use raw GroupBy query for aggregation
	var rows []struct {
		JetonID string `json:"jeton_id"`
		Count   int    `json:"count"`
	}
	err := r.ec(ctx).Product.Query().
		Where(
			product.JetonIDIn(ids...),
			product.TypeEQ(product.TypeSimple),
		).
		GroupBy(product.FieldJetonID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, translateError(err)
	}

	for _, row := range rows {
		result[row.JetonID] = int64(row.Count)
	}
	return result, nil
}

func (r *ProductRepository) UpdateJeton(ctx context.Context, id string, jetonID *string) error {
	builder := r.ec(ctx).Product.UpdateOneID(id)
	if jetonID != nil {
		builder.SetJetonID(*jetonID)
	} else {
		builder.ClearJetonID()
	}
	_, err := builder.Save(ctx)
	return translateError(err)
}

// productNameContainsILIKE provides ILIKE search via sql modifier.
// This is used when the generated NameContainsFold is not sufficient.
var _ = func() sql.Querier { return nil } // import anchor
