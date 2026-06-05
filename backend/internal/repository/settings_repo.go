package repository

import (
	"context"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/club100freeproduct"
	"backend/internal/generated/ent/settings"
	"backend/internal/generated/ent/tenant"

	"github.com/google/uuid"
)

type SettingsRepository interface {
	Get(ctx context.Context) (*ent.Settings, error)
	GetWithProducts(ctx context.Context) (*ent.Settings, error)
	Upsert(ctx context.Context, mode settings.PosMode) error
	UpdateClub100Settings(ctx context.Context, freeProductIDs []uuid.UUID, maxRedemptions int) error
	IsSystemEnabled(ctx context.Context) (bool, error)
	SetSystemEnabled(ctx context.Context, enabled bool) error
	UpdateBranding(ctx context.Context, b BrandingUpdate) error
	// TenantName returns the current tenant's name (RLS scopes it to one row).
	TenantName(ctx context.Context) (string, error)
}

// BrandingUpdate carries branding columns to upsert. A nil pointer leaves the
// column unchanged; a non-nil pointer to "" clears a nillable column (NULL).
type BrandingUpdate struct {
	DisplayName            *string
	LogoURL                *string
	FaviconURL             *string
	ColorPrimary           *string
	ColorPrimaryForeground *string
	ColorBackground        *string
	ColorForeground        *string
	ContactEmail           *string
	ContactPhone           *string
	LegalName              *string
	LegalAddress           *string
	LegalText              *string
	Currency               *string
	PaymentProvider        *string
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

// defaultSettings returns the in-memory settings used before a tenant's row is
// created. Branding columns mirror the Ent schema defaults so the API renders
// neutral values until the tenant customizes them.
func defaultSettings(id string) *ent.Settings {
	return &ent.Settings{
		ID:                     id,
		PosMode:                settings.PosModeQR_CODE,
		SystemEnabled:          true,
		Club100MaxRedemptions:  2,
		ColorPrimary:           "#f87778",
		ColorPrimaryForeground: "#fdfdfd",
		ColorBackground:        "#e9e7e6",
		ColorForeground:        "#000000",
		Currency:               "CHF",
		PaymentProvider:        "payrexx",
	}
}

func (r *settingsRepo) Get(ctx context.Context) (*ent.Settings, error) {
	id := r.settingsID(ctx)
	e, err := r.ec(ctx).Settings.Get(ctx, id)
	if err != nil {
		if translateError(err) == ErrNotFound {
			return defaultSettings(id), nil
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
			return defaultSettings(id), nil
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

func (r *settingsRepo) TenantName(ctx context.Context) (string, error) {
	tenantID, ok := TenantIDFromContext(ctx)
	if !ok {
		return "", ErrNotFound
	}
	t, err := r.ec(ctx).Tenant.Query().Where(tenant.IDEQ(tenantID)).Only(ctx)
	if err != nil {
		return "", translateError(err)
	}
	return t.Name, nil
}

func (r *settingsRepo) UpdateBranding(ctx context.Context, b BrandingUpdate) error {
	id := r.settingsID(ctx)
	create := r.ec(ctx).Settings.Create().SetID(id)
	if tenantID, ok := TenantIDFromContext(ctx); ok {
		create = create.SetTenantID(tenantID)
	}

	var upserts []func(*ent.SettingsUpsert)

	addNillable := func(v *string, set func(string), setU func(string), clearU func()) {
		if v == nil {
			return
		}
		if *v == "" {
			upserts = append(upserts, func(*ent.SettingsUpsert) { clearU() })
			return
		}
		set(*v)
		val := *v
		upserts = append(upserts, func(*ent.SettingsUpsert) { setU(val) })
	}
	addRequired := func(v *string, set func(string), setU func(string)) {
		if v == nil {
			return
		}
		set(*v)
		val := *v
		upserts = append(upserts, func(*ent.SettingsUpsert) { setU(val) })
	}

	// Hold the upsert receiver so the closures can target it.
	var up *ent.SettingsUpsert

	addNillable(b.DisplayName,
		func(s string) { create.SetDisplayName(s) },
		func(s string) { up.SetDisplayName(s) },
		func() { up.ClearDisplayName() })
	addNillable(b.LogoURL,
		func(s string) { create.SetLogoURL(s) },
		func(s string) { up.SetLogoURL(s) },
		func() { up.ClearLogoURL() })
	addNillable(b.FaviconURL,
		func(s string) { create.SetFaviconURL(s) },
		func(s string) { up.SetFaviconURL(s) },
		func() { up.ClearFaviconURL() })
	addNillable(b.ContactEmail,
		func(s string) { create.SetContactEmail(s) },
		func(s string) { up.SetContactEmail(s) },
		func() { up.ClearContactEmail() })
	addNillable(b.ContactPhone,
		func(s string) { create.SetContactPhone(s) },
		func(s string) { up.SetContactPhone(s) },
		func() { up.ClearContactPhone() })
	addNillable(b.LegalName,
		func(s string) { create.SetLegalName(s) },
		func(s string) { up.SetLegalName(s) },
		func() { up.ClearLegalName() })
	addNillable(b.LegalAddress,
		func(s string) { create.SetLegalAddress(s) },
		func(s string) { up.SetLegalAddress(s) },
		func() { up.ClearLegalAddress() })
	addNillable(b.LegalText,
		func(s string) { create.SetLegalText(s) },
		func(s string) { up.SetLegalText(s) },
		func() { up.ClearLegalText() })

	addRequired(b.ColorPrimary,
		func(s string) { create.SetColorPrimary(s) },
		func(s string) { up.SetColorPrimary(s) })
	addRequired(b.ColorPrimaryForeground,
		func(s string) { create.SetColorPrimaryForeground(s) },
		func(s string) { up.SetColorPrimaryForeground(s) })
	addRequired(b.ColorBackground,
		func(s string) { create.SetColorBackground(s) },
		func(s string) { up.SetColorBackground(s) })
	addRequired(b.ColorForeground,
		func(s string) { create.SetColorForeground(s) },
		func(s string) { up.SetColorForeground(s) })
	addRequired(b.Currency,
		func(s string) { create.SetCurrency(s) },
		func(s string) { up.SetCurrency(s) })
	addRequired(b.PaymentProvider,
		func(s string) { create.SetPaymentProvider(s) },
		func(s string) { up.SetPaymentProvider(s) })

	if len(upserts) == 0 {
		return nil
	}

	err := create.
		OnConflictColumns(settings.FieldID).
		Update(func(u *ent.SettingsUpsert) {
			up = u
			for _, fn := range upserts {
				fn(u)
			}
		}).
		Exec(ctx)
	return translateError(err)
}
