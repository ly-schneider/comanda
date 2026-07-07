package repository

import (
	"context"
	"encoding/json"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenantplugin"
)

type TenantPluginUpsert struct {
	PluginKey           string
	Enabled             bool
	Config              json.RawMessage
	SecretsEnc          []byte
	SecretsConfiguredAt *time.Time
}

type TenantPluginRepository interface {
	GetByKey(ctx context.Context, pluginKey string) (*ent.TenantPlugin, error)
	ListEnabled(ctx context.Context) ([]*ent.TenantPlugin, error)
	ListByTenant(ctx context.Context, tenantID string) ([]*ent.TenantPlugin, error)
	Upsert(ctx context.Context, tenantID string, in TenantPluginUpsert) (*ent.TenantPlugin, error)
}

type tenantPluginRepo struct {
	client *ent.Client
}

func NewTenantPluginRepository(client *ent.Client) TenantPluginRepository {
	return &tenantPluginRepo{client: client}
}

func (r *tenantPluginRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *tenantPluginRepo) GetByKey(ctx context.Context, pluginKey string) (*ent.TenantPlugin, error) {
	row, err := r.ec(ctx).TenantPlugin.Query().
		Where(tenantplugin.PluginKeyEQ(pluginKey)).
		Only(ctx)
	return row, translateError(err)
}

func (r *tenantPluginRepo) ListEnabled(ctx context.Context) ([]*ent.TenantPlugin, error) {
	rows, err := r.ec(ctx).TenantPlugin.Query().
		Where(tenantplugin.EnabledEQ(true)).
		All(ctx)
	return rows, translateError(err)
}

func (r *tenantPluginRepo) ListByTenant(ctx context.Context, tenantID string) ([]*ent.TenantPlugin, error) {
	rows, err := r.ec(ctx).TenantPlugin.Query().
		Where(tenantplugin.TenantIDEQ(tenantID)).
		All(ctx)
	return rows, translateError(err)
}

func (r *tenantPluginRepo) Upsert(ctx context.Context, tenantID string, in TenantPluginUpsert) (*ent.TenantPlugin, error) {
	create := r.ec(ctx).TenantPlugin.Create().
		SetTenantID(tenantID).
		SetPluginKey(in.PluginKey).
		SetEnabled(in.Enabled).
		SetConfig(in.Config)
	if in.SecretsEnc != nil {
		create = create.SetSecretsEnc(in.SecretsEnc)
		if in.SecretsConfiguredAt != nil {
			create = create.SetSecretsConfiguredAt(*in.SecretsConfiguredAt)
		}
	}

	upsert := create.
		OnConflictColumns(tenantplugin.FieldTenantID, tenantplugin.FieldPluginKey).
		SetEnabled(in.Enabled).
		SetConfig(in.Config).
		UpdateUpdatedAt()
	if in.SecretsEnc != nil {
		upsert = upsert.SetSecretsEnc(in.SecretsEnc)
		if in.SecretsConfiguredAt != nil {
			upsert = upsert.SetSecretsConfiguredAt(*in.SecretsConfiguredAt)
		}
	}
	if err := upsert.Exec(ctx); err != nil {
		return nil, translateError(err)
	}

	row, err := r.ec(ctx).TenantPlugin.Query().
		Where(
			tenantplugin.TenantIDEQ(tenantID),
			tenantplugin.PluginKeyEQ(in.PluginKey),
		).
		Only(ctx)
	return row, translateError(err)
}
