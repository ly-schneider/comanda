package repository

import (
	"context"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/club100freeproduct"
	"backend/internal/generated/ent/settings"

	"github.com/google/uuid"
)

type SettingsRepository interface {
	Get(ctx context.Context) (*ent.Settings, error)
	GetWithProducts(ctx context.Context) (*ent.Settings, error)
	Upsert(ctx context.Context, mode settings.PosMode) error
	UpdateClub100Settings(ctx context.Context, freeProductIDs []uuid.UUID, maxRedemptions int) error
	IsSystemEnabled(ctx context.Context) (bool, error)
	SetSystemEnabled(ctx context.Context, enabled bool) error
}

type settingsRepo struct {
	client *ent.Client
}

func NewSettingsRepository(client *ent.Client) SettingsRepository {
	return &settingsRepo{client: client}
}

func (r *settingsRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

// settingsID returns the per-tenant settings row id. Settings is a singleton
// per tenant; its primary key is the tenant's UUID string so it never collides
// across tenants while RLS still scopes visibility by tenant_id.
func (r *settingsRepo) settingsID(ctx context.Context) string {
	if id, ok := TenantIDFromContext(ctx); ok {
		return id.String()
	}
	return "default"
}

func (r *settingsRepo) Get(ctx context.Context) (*ent.Settings, error) {
	id := r.settingsID(ctx)
	e, err := r.ec(ctx).Settings.Get(ctx, id)
	if err != nil {
		if translateError(err) == ErrNotFound {
			return &ent.Settings{
				ID:                    id,
				PosMode:               settings.PosModeQR_CODE,
				SystemEnabled:         true,
				Club100MaxRedemptions: 2,
			}, nil
		}
		return nil, translateError(err)
	}
	return e, nil
}

func (r *settingsRepo) GetWithProducts(ctx context.Context) (*ent.Settings, error) {
	id := r.settingsID(ctx)
	e, err := r.ec(ctx).Settings.Query().
		Where(settings.IDEQ(id)).
		WithClub100FreeProducts().
		Only(ctx)
	if err != nil {
		if translateError(err) == ErrNotFound {
			return &ent.Settings{
				ID:                    id,
				PosMode:               settings.PosModeQR_CODE,
				SystemEnabled:         true,
				Club100MaxRedemptions: 2,
			}, nil
		}
		return nil, translateError(err)
	}
	return e, nil
}

func (r *settingsRepo) Upsert(ctx context.Context, mode settings.PosMode) error {
	id := r.settingsID(ctx)
	create := r.ec(ctx).Settings.Create().
		SetID(id).
		SetPosMode(mode)
	if tenantID, ok := TenantIDFromContext(ctx); ok {
		create = create.SetTenantID(tenantID)
	}
	err := create.
		OnConflictColumns(settings.FieldID).
		SetPosMode(mode).
		Exec(ctx)
	return translateError(err)
}

func (r *settingsRepo) UpdateClub100Settings(ctx context.Context, freeProductIDs []uuid.UUID, maxRedemptions int) error {
	client := r.ec(ctx)
	id := r.settingsID(ctx)
	tenantID, hasTenant := TenantIDFromContext(ctx)

	create := client.Settings.Create().
		SetID(id).
		SetClub100MaxRedemptions(maxRedemptions)
	if hasTenant {
		create = create.SetTenantID(tenantID)
	}
	err := create.
		OnConflictColumns(settings.FieldID).
		SetClub100MaxRedemptions(maxRedemptions).
		Exec(ctx)
	if err != nil {
		return translateError(err)
	}

	_, err = client.Club100FreeProduct.Delete().
		Where(club100freeproduct.SettingsIDEQ(id)).
		Exec(ctx)
	if err != nil {
		return translateError(err)
	}

	for _, productID := range freeProductIDs {
		fpCreate := client.Club100FreeProduct.Create().
			SetSettingsID(id).
			SetProductID(productID)
		if hasTenant {
			fpCreate = fpCreate.SetTenantID(tenantID)
		}
		if err := fpCreate.Exec(ctx); err != nil {
			return translateError(err)
		}
	}

	return nil
}

func (r *settingsRepo) IsSystemEnabled(ctx context.Context) (bool, error) {
	s, err := r.Get(ctx)
	if err != nil {
		return true, err
	}
	return s.SystemEnabled, nil
}

func (r *settingsRepo) SetSystemEnabled(ctx context.Context, enabled bool) error {
	id := r.settingsID(ctx)
	create := r.ec(ctx).Settings.Create().
		SetID(id).
		SetSystemEnabled(enabled)
	if tenantID, ok := TenantIDFromContext(ctx); ok {
		create = create.SetTenantID(tenantID)
	}
	err := create.
		OnConflictColumns(settings.FieldID).
		SetSystemEnabled(enabled).
		Exec(ctx)
	return translateError(err)
}
