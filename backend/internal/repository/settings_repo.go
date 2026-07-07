package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/settings"
	"backend/internal/generated/ent/tenant"

	"entgo.io/ent/dialect/sql"
)

type SettingsRepository interface {
	Get(ctx context.Context) (*ent.Settings, error)
	GetWithProducts(ctx context.Context) (*ent.Settings, error)
	Upsert(ctx context.Context, mode settings.PosMode) error
	IsSystemEnabled(ctx context.Context) (bool, error)
	SetSystemEnabled(ctx context.Context, enabled bool) error
	// SetOnboardingCompleted stamps (completed=true) or clears the onboarding
	// completion time. Per-step progress is derived elsewhere from real data.
	SetOnboardingCompleted(ctx context.Context, completed bool) error
	UpdateBranding(ctx context.Context, b BrandingUpdate) error
	// TenantName returns the current tenant's name (RLS scopes it to one row).
	TenantName(ctx context.Context) (string, error)
	// EnsureWebhookToken returns the tenant's webhook token, generating one if absent.
	EnsureWebhookToken(ctx context.Context) (string, error)
	UpsertPayrexxCredentials(ctx context.Context, c PayrexxCredentials) error
	// UpsertPayrexxAPICredentials stores the instance + API key + webhook token
	// without the webhook secret, leaving the tenant in the "API connected"
	// state (payrexx_configured_at stays unset until the webhook secret lands).
	UpsertPayrexxAPICredentials(ctx context.Context, instance, apiSecretEnc, webhookToken string) error
	// SetPayrexxWebhookSecret stores the webhook secret and stamps
	// payrexx_configured_at; returns ErrNotFound if the API step hasn't run yet.
	SetPayrexxWebhookSecret(ctx context.Context, webhookSecretEnc string) error
	// TouchPayrexxWebhookReceived stamps payrexx_last_webhook_at = now.
	TouchPayrexxWebhookReceived(ctx context.Context) error
	// GetPayrexxConfig returns the non-secret view driving the staged setup UI;
	// an empty value (not an error) when nothing is configured.
	GetPayrexxConfig(ctx context.Context) (*PayrexxConfig, error)
	// GetPayrexxAPICredentials returns the instance + encrypted API key without
	// requiring the webhook secret; ErrNotFound when the API step hasn't run.
	GetPayrexxAPICredentials(ctx context.Context) (instance, apiSecretEnc string, err error)
	// GetPayrexxCredentials returns ErrNotFound when the tenant has none configured.
	GetPayrexxCredentials(ctx context.Context) (*PayrexxCredentials, error)
	UpsertPlunkCredentials(ctx context.Context, c PlunkCredentials) error
	// GetPlunkCredentials returns ErrNotFound when the tenant has none configured.
	GetPlunkCredentials(ctx context.Context) (*PlunkCredentials, error)
	// GetQRSigningKey returns the tenant's stored Ed25519 keypair material, or
	// ErrNotFound when none has been generated yet.
	GetQRSigningKey(ctx context.Context) (*QRSigningKey, error)
	UpsertQRSigningKey(ctx context.Context, k QRSigningKey) error
}

// QRSigningKey carries the per-tenant Ed25519 signing material. PrivateSeedEnc
// is AES-256-GCM ciphertext (base64) of the 32-byte seed; PublicKey is
// base64url(no-pad) of the 32-byte public key (plaintext, public by design).
type QRSigningKey struct {
	PrivateSeedEnc string
	PublicKey      string
}

// PayrexxCredentials carries the per-tenant Payrexx connection. Instance is
// plaintext (part of the API URL); the secrets are AES-256-GCM ciphertext.
type PayrexxCredentials struct {
	Instance         string
	APISecretEnc     string
	WebhookSecretEnc string
	WebhookToken     string
}

// PayrexxConfig is the non-secret view of a tenant's Payrexx connection that
// drives the staged setup UI. Secrets are reported as presence booleans only.
type PayrexxConfig struct {
	Instance         string
	HasAPISecret     bool
	HasWebhookSecret bool
	WebhookToken     string
	ConfiguredAt     *time.Time
	LastWebhookAt    *time.Time
}

type PlunkCredentials struct {
	APIKeyEnc string
	FromName  string
	FromEmail string
	ReplyTo   string
}

// BrandingUpdate carries branding columns to upsert. A nil pointer leaves the
// column unchanged; a non-nil pointer to "" clears a nillable column (NULL).
type BrandingUpdate struct {
	DisplayName            *string
	LogoURL                *string
	FaviconURL             *string
	ColorPrimary           *string
	ColorPrimaryForeground *string
	ColorSecondary         *string
	ColorBackground        *string
	ColorForeground        *string
	FontKey                *string
	RadiusKey              *string
	ContactEmail           *string
	ContactPhone           *string
	LegalName              *string
	LegalAddress           *string
	Currency               *string
	PaymentProvider        *string
	SumupMerchantCode      *string
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
// per tenant; its primary key is the tenant's id so it never collides
// across tenants while RLS still scopes visibility by tenant_id.
func (r *settingsRepo) settingsID(ctx context.Context) string {
	if id, ok := TenantIDFromContext(ctx); ok {
		return id
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
		ColorPrimary:           "#0a0a0a",
		ColorPrimaryForeground: "#ffffff",
		ColorSecondary:         "#f5f5f5",
		ColorBackground:        "#ffffff",
		ColorForeground:        "#0a0a0a",
		FontKey:                "inter",
		RadiusKey:              "rounded",
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

func (r *settingsRepo) SetOnboardingCompleted(ctx context.Context, completed bool) error {
	id := r.settingsID(ctx)
	create := r.ec(ctx).Settings.Create().SetID(id)
	if tenantID, ok := TenantIDFromContext(ctx); ok {
		create = create.SetTenantID(tenantID)
	}
	if completed {
		now := time.Now()
		create = create.SetOnboardingCompletedAt(now)
		return translateError(create.
			OnConflictColumns(settings.FieldID).
			Update(func(u *ent.SettingsUpsert) { u.SetOnboardingCompletedAt(now) }).
			Exec(ctx))
	}
	return translateError(create.
		OnConflictColumns(settings.FieldID).
		Update(func(u *ent.SettingsUpsert) { u.ClearOnboardingCompletedAt() }).
		Exec(ctx))
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
	addNillable(b.SumupMerchantCode,
		func(s string) { create.SetSumupMerchantCode(s) },
		func(s string) { up.SetSumupMerchantCode(s) },
		func() { up.ClearSumupMerchantCode() })

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
	addRequired(b.ColorSecondary,
		func(s string) { create.SetColorSecondary(s) },
		func(s string) { up.SetColorSecondary(s) })
	addRequired(b.FontKey,
		func(s string) { create.SetFontKey(s) },
		func(s string) { up.SetFontKey(s) })
	addRequired(b.RadiusKey,
		func(s string) { create.SetRadiusKey(s) },
		func(s string) { up.SetRadiusKey(s) })
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

func newWebhookToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (r *settingsRepo) EnsureWebhookToken(ctx context.Context) (string, error) {
	id := r.settingsID(ctx)
	tenantID, hasTenant := TenantIDFromContext(ctx)

	e, err := r.ec(ctx).Settings.Get(ctx, id)
	if err == nil && e.PaymentWebhookToken != nil && *e.PaymentWebhookToken != "" {
		return *e.PaymentWebhookToken, nil
	}
	if err != nil && translateError(err) != ErrNotFound {
		return "", translateError(err)
	}

	token, err := newWebhookToken()
	if err != nil {
		return "", err
	}

	create := r.ec(ctx).Settings.Create().
		SetID(id).
		SetPaymentWebhookToken(token)
	if hasTenant {
		create = create.SetTenantID(tenantID)
	}
	// COALESCE keeps any value already stored so a concurrent caller can't
	// clobber a token that was already handed out.
	if err := create.
		OnConflict(
			sql.ConflictColumns(settings.FieldID),
			sql.ResolveWith(func(u *sql.UpdateSet) {
				u.Set(settings.FieldPaymentWebhookToken, sql.Expr(
					"COALESCE("+settings.Table+"."+settings.FieldPaymentWebhookToken+", EXCLUDED."+settings.FieldPaymentWebhookToken+")",
				))
			}),
		).
		Exec(ctx); err != nil {
		return "", translateError(err)
	}
	stored, err := r.ec(ctx).Settings.Get(ctx, id)
	if err != nil {
		return "", translateError(err)
	}
	if stored.PaymentWebhookToken == nil || *stored.PaymentWebhookToken == "" {
		return token, nil
	}
	return *stored.PaymentWebhookToken, nil
}

func (r *settingsRepo) UpsertPayrexxCredentials(ctx context.Context, c PayrexxCredentials) error {
	id := r.settingsID(ctx)
	tenantID, hasTenant := TenantIDFromContext(ctx)
	now := time.Now()

	create := r.ec(ctx).Settings.Create().
		SetID(id).
		SetPayrexxInstance(c.Instance).
		SetPayrexxAPISecretEnc(c.APISecretEnc).
		SetPayrexxWebhookSecretEnc(c.WebhookSecretEnc).
		SetPaymentWebhookToken(c.WebhookToken).
		SetPaymentProvider("payrexx").
		SetPayrexxConfiguredAt(now)
	if hasTenant {
		create = create.SetTenantID(tenantID)
	}

	err := create.
		OnConflictColumns(settings.FieldID).
		Update(func(u *ent.SettingsUpsert) {
			u.SetPayrexxInstance(c.Instance)
			u.SetPayrexxAPISecretEnc(c.APISecretEnc)
			u.SetPayrexxWebhookSecretEnc(c.WebhookSecretEnc)
			u.SetPaymentWebhookToken(c.WebhookToken)
			u.SetPaymentProvider("payrexx")
			u.SetPayrexxConfiguredAt(now)
		}).
		Exec(ctx)
	return translateError(err)
}

func (r *settingsRepo) GetPayrexxCredentials(ctx context.Context) (*PayrexxCredentials, error) {
	id := r.settingsID(ctx)
	e, err := r.ec(ctx).Settings.Get(ctx, id)
	if err != nil {
		return nil, translateError(err)
	}
	if e.PayrexxInstance == nil || e.PayrexxAPISecretEnc == nil || e.PayrexxWebhookSecretEnc == nil {
		return nil, ErrNotFound
	}
	token := ""
	if e.PaymentWebhookToken != nil {
		token = *e.PaymentWebhookToken
	}
	return &PayrexxCredentials{
		Instance:         *e.PayrexxInstance,
		APISecretEnc:     *e.PayrexxAPISecretEnc,
		WebhookSecretEnc: *e.PayrexxWebhookSecretEnc,
		WebhookToken:     token,
	}, nil
}

func (r *settingsRepo) UpsertPayrexxAPICredentials(ctx context.Context, instance, apiSecretEnc, webhookToken string) error {
	id := r.settingsID(ctx)
	tenantID, hasTenant := TenantIDFromContext(ctx)

	create := r.ec(ctx).Settings.Create().
		SetID(id).
		SetPayrexxInstance(instance).
		SetPayrexxAPISecretEnc(apiSecretEnc).
		SetPaymentWebhookToken(webhookToken).
		SetPaymentProvider("payrexx")
	if hasTenant {
		create = create.SetTenantID(tenantID)
	}

	// Intentionally leaves payrexx_webhook_secret_enc and payrexx_configured_at
	// untouched so re-running the API step never clears an existing webhook secret.
	err := create.
		OnConflictColumns(settings.FieldID).
		Update(func(u *ent.SettingsUpsert) {
			u.SetPayrexxInstance(instance)
			u.SetPayrexxAPISecretEnc(apiSecretEnc)
			u.SetPaymentWebhookToken(webhookToken)
			u.SetPaymentProvider("payrexx")
		}).
		Exec(ctx)
	return translateError(err)
}

func (r *settingsRepo) SetPayrexxWebhookSecret(ctx context.Context, webhookSecretEnc string) error {
	id := r.settingsID(ctx)
	e, err := r.ec(ctx).Settings.Get(ctx, id)
	if err != nil {
		return translateError(err)
	}
	if e.PayrexxInstance == nil || e.PayrexxAPISecretEnc == nil {
		return ErrNotFound
	}
	_, err = r.ec(ctx).Settings.UpdateOneID(id).
		SetPayrexxWebhookSecretEnc(webhookSecretEnc).
		SetPayrexxConfiguredAt(time.Now()).
		Save(ctx)
	return translateError(err)
}

func (r *settingsRepo) TouchPayrexxWebhookReceived(ctx context.Context) error {
	_, err := r.ec(ctx).Settings.UpdateOneID(r.settingsID(ctx)).
		SetPayrexxLastWebhookAt(time.Now()).
		Save(ctx)
	return translateError(err)
}

func (r *settingsRepo) GetPayrexxConfig(ctx context.Context) (*PayrexxConfig, error) {
	e, err := r.ec(ctx).Settings.Get(ctx, r.settingsID(ctx))
	if err != nil {
		if translateError(err) == ErrNotFound {
			return &PayrexxConfig{}, nil
		}
		return nil, translateError(err)
	}
	cfg := &PayrexxConfig{
		HasAPISecret:     e.PayrexxAPISecretEnc != nil,
		HasWebhookSecret: e.PayrexxWebhookSecretEnc != nil,
		ConfiguredAt:     e.PayrexxConfiguredAt,
		LastWebhookAt:    e.PayrexxLastWebhookAt,
	}
	if e.PayrexxInstance != nil {
		cfg.Instance = *e.PayrexxInstance
	}
	if e.PaymentWebhookToken != nil {
		cfg.WebhookToken = *e.PaymentWebhookToken
	}
	return cfg, nil
}

func (r *settingsRepo) GetPayrexxAPICredentials(ctx context.Context) (string, string, error) {
	e, err := r.ec(ctx).Settings.Get(ctx, r.settingsID(ctx))
	if err != nil {
		return "", "", translateError(err)
	}
	if e.PayrexxInstance == nil || e.PayrexxAPISecretEnc == nil {
		return "", "", ErrNotFound
	}
	return *e.PayrexxInstance, *e.PayrexxAPISecretEnc, nil
}

func (r *settingsRepo) GetQRSigningKey(ctx context.Context) (*QRSigningKey, error) {
	id := r.settingsID(ctx)
	e, err := r.ec(ctx).Settings.Get(ctx, id)
	if err != nil {
		return nil, translateError(err)
	}
	if e.Ed25519PrivateSeedEnc == nil || e.Ed25519PublicKey == nil {
		return nil, ErrNotFound
	}
	return &QRSigningKey{
		PrivateSeedEnc: *e.Ed25519PrivateSeedEnc,
		PublicKey:      *e.Ed25519PublicKey,
	}, nil
}

func (r *settingsRepo) UpsertQRSigningKey(ctx context.Context, k QRSigningKey) error {
	id := r.settingsID(ctx)
	tenantID, hasTenant := TenantIDFromContext(ctx)

	create := r.ec(ctx).Settings.Create().
		SetID(id).
		SetEd25519PrivateSeedEnc(k.PrivateSeedEnc).
		SetEd25519PublicKey(k.PublicKey)
	if hasTenant {
		create = create.SetTenantID(tenantID)
	}

	// COALESCE keeps any key already stored so a concurrent caller can't clobber
	// a keypair that was already handed out (the public key is committed-to).
	err := create.
		OnConflict(
			sql.ConflictColumns(settings.FieldID),
			sql.ResolveWith(func(u *sql.UpdateSet) {
				u.Set(settings.FieldEd25519PrivateSeedEnc, sql.Expr(
					"COALESCE("+settings.Table+"."+settings.FieldEd25519PrivateSeedEnc+", EXCLUDED."+settings.FieldEd25519PrivateSeedEnc+")",
				))
				u.Set(settings.FieldEd25519PublicKey, sql.Expr(
					"COALESCE("+settings.Table+"."+settings.FieldEd25519PublicKey+", EXCLUDED."+settings.FieldEd25519PublicKey+")",
				))
			}),
		).
		Exec(ctx)
	return translateError(err)
}

func (r *settingsRepo) UpsertPlunkCredentials(ctx context.Context, c PlunkCredentials) error {
	id := r.settingsID(ctx)
	tenantID, hasTenant := TenantIDFromContext(ctx)
	now := time.Now()

	create := r.ec(ctx).Settings.Create().
		SetID(id).
		SetPlunkAPIKeyEnc(c.APIKeyEnc).
		SetPlunkFromName(c.FromName).
		SetPlunkFromEmail(c.FromEmail).
		SetPlunkReplyTo(c.ReplyTo).
		SetPlunkConfiguredAt(now)
	if hasTenant {
		create = create.SetTenantID(tenantID)
	}

	err := create.
		OnConflictColumns(settings.FieldID).
		Update(func(u *ent.SettingsUpsert) {
			u.SetPlunkAPIKeyEnc(c.APIKeyEnc)
			u.SetPlunkFromName(c.FromName)
			u.SetPlunkFromEmail(c.FromEmail)
			u.SetPlunkReplyTo(c.ReplyTo)
			u.SetPlunkConfiguredAt(now)
		}).
		Exec(ctx)
	return translateError(err)
}

func (r *settingsRepo) GetPlunkCredentials(ctx context.Context) (*PlunkCredentials, error) {
	id := r.settingsID(ctx)
	e, err := r.ec(ctx).Settings.Get(ctx, id)
	if err != nil {
		return nil, translateError(err)
	}
	if e.PlunkAPIKeyEnc == nil {
		return nil, ErrNotFound
	}
	out := &PlunkCredentials{APIKeyEnc: *e.PlunkAPIKeyEnc}
	if e.PlunkFromName != nil {
		out.FromName = *e.PlunkFromName
	}
	if e.PlunkFromEmail != nil {
		out.FromEmail = *e.PlunkFromEmail
	}
	if e.PlunkReplyTo != nil {
		out.ReplyTo = *e.PlunkReplyTo
	}
	return out, nil
}
