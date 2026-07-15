package stationqr

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/stationenrollpreset"
	"backend/internal/generated/ent/stationpresetproduct"
	"backend/internal/repository"

	"entgo.io/ent/dialect/sql"
)

type PresetRepository interface {
	Create(ctx context.Context, name string, tokenEnc []byte, issuedBy string) (*ent.StationEnrollPreset, error)
	List(ctx context.Context) ([]*ent.StationEnrollPreset, error)
	GetByID(ctx context.Context, id string) (*ent.StationEnrollPreset, error)
	Rename(ctx context.Context, id, name string) (*ent.StationEnrollPreset, error)
	SetToken(ctx context.Context, id string, tokenEnc []byte) (*ent.StationEnrollPreset, error)
	SetDisabled(ctx context.Context, id string, disabled bool) (*ent.StationEnrollPreset, error)
	Delete(ctx context.Context, id string) error

	ReplaceProducts(ctx context.Context, presetID string, productIDs []string) error
	ListProducts(ctx context.Context, presetID string) ([]*ent.StationPresetProduct, error)
	ListAllProducts(ctx context.Context) ([]*ent.StationPresetProduct, error)
}

type presetRepo struct {
	client *ent.Client
}

func NewPresetRepository(client *ent.Client) PresetRepository {
	return &presetRepo{client: client}
}

func (r *presetRepo) ec(ctx context.Context) *ent.Client {
	return repository.ClientFromContext(ctx, r.client)
}

func (r *presetRepo) Create(ctx context.Context, name string, tokenEnc []byte, issuedBy string) (*ent.StationEnrollPreset, error) {
	created, err := r.ec(ctx).StationEnrollPreset.Create().
		SetName(name).
		SetTokenEnc(tokenEnc).
		SetIssuedBy(issuedBy).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return created, nil
}

func (r *presetRepo) List(ctx context.Context) ([]*ent.StationEnrollPreset, error) {
	rows, err := r.ec(ctx).StationEnrollPreset.Query().
		Order(stationenrollpreset.ByCreatedAt(sql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}

func (r *presetRepo) GetByID(ctx context.Context, id string) (*ent.StationEnrollPreset, error) {
	row, err := r.ec(ctx).StationEnrollPreset.Get(ctx, id)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return row, nil
}

func (r *presetRepo) Rename(ctx context.Context, id, name string) (*ent.StationEnrollPreset, error) {
	updated, err := r.ec(ctx).StationEnrollPreset.UpdateOneID(id).
		SetName(name).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return updated, nil
}

func (r *presetRepo) SetToken(ctx context.Context, id string, tokenEnc []byte) (*ent.StationEnrollPreset, error) {
	updated, err := r.ec(ctx).StationEnrollPreset.UpdateOneID(id).
		SetTokenEnc(tokenEnc).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return updated, nil
}

func (r *presetRepo) SetDisabled(ctx context.Context, id string, disabled bool) (*ent.StationEnrollPreset, error) {
	b := r.ec(ctx).StationEnrollPreset.UpdateOneID(id)
	if disabled {
		b.SetDisabledAt(time.Now().UTC())
	} else {
		b.ClearDisabledAt()
	}
	updated, err := b.Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return updated, nil
}

func (r *presetRepo) Delete(ctx context.Context, id string) error {
	err := r.ec(ctx).StationEnrollPreset.DeleteOneID(id).Exec(ctx)
	return repository.TranslateError(err)
}

func (r *presetRepo) ReplaceProducts(ctx context.Context, presetID string, productIDs []string) error {
	// Runs on the request-scoped tenant tx; the delete+insert is atomic through
	// that outer transaction. A nested Ent Tx() here would bypass app.tenant_id.
	client := r.ec(ctx)

	if _, err := client.StationPresetProduct.Delete().
		Where(stationpresetproduct.PresetID(presetID)).
		Exec(ctx); err != nil {
		return repository.TranslateError(err)
	}

	if len(productIDs) == 0 {
		return nil
	}
	builders := make([]*ent.StationPresetProductCreate, len(productIDs))
	for i, pid := range productIDs {
		builders[i] = client.StationPresetProduct.Create().
			SetPresetID(presetID).
			SetProductID(pid)
	}
	if _, err := client.StationPresetProduct.CreateBulk(builders...).Save(ctx); err != nil {
		return repository.TranslateError(err)
	}
	return nil
}

func (r *presetRepo) ListProducts(ctx context.Context, presetID string) ([]*ent.StationPresetProduct, error) {
	rows, err := r.ec(ctx).StationPresetProduct.Query().
		Where(stationpresetproduct.PresetID(presetID)).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}

func (r *presetRepo) ListAllProducts(ctx context.Context) ([]*ent.StationPresetProduct, error) {
	rows, err := r.ec(ctx).StationPresetProduct.Query().All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}
