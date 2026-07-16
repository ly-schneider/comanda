package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type Settings struct {
	ent.Schema
}

func (Settings) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "settings"},
	}
}

func (Settings) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (Settings) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(50).
			Immutable(),
		field.Enum("pos_mode").
			Values("QR_CODE", "JETON", "HYBRID").
			Default("QR_CODE").
			StorageKey("pos_mode"),
		field.Bool("system_enabled").
			Default(true),
		field.Bool("station_self_manage_products").
			Default(false),
		field.String("display_name").
			MaxLen(100).
			Optional().
			Nillable(),
		field.Text("logo_url").
			Optional().
			Nillable(),
		field.Text("favicon_url").
			Optional().
			Nillable(),
		field.String("color_primary").
			MaxLen(9).
			Default("#0a0a0a"),
		field.String("color_primary_foreground").
			MaxLen(9).
			Default("#ffffff"),
		field.String("color_secondary").
			MaxLen(9).
			Default("#f5f5f5"),
		field.String("color_background").
			MaxLen(9).
			Default("#ffffff"),
		field.String("color_foreground").
			MaxLen(9).
			Default("#0a0a0a"),
		// Curated font + corner-style keys; validated against an allow-list in the
		// service before the web maps each key to a constant (never raw CSS).
		field.String("font_key").
			MaxLen(32).
			Default("inter"),
		field.String("radius_key").
			MaxLen(16).
			Default("rounded"),
		field.String("contact_email").
			MaxLen(255).
			Optional().
			Nillable(),
		field.String("contact_phone").
			MaxLen(50).
			Optional().
			Nillable(),
		field.String("legal_name").
			MaxLen(200).
			Optional().
			Nillable(),
		field.Text("legal_address").
			Optional().
			Nillable(),
		field.String("currency").
			MaxLen(3).
			Default("CHF"),
		field.String("payment_provider").
			MaxLen(50).
			Default("payrexx"),
		// Identifier, not a credential — stored plaintext (unlike payrexx_*_enc).
		field.String("sumup_merchant_code").
			MaxLen(255).
			Optional().
			Nillable(),
		// Opaque token routing inbound webhooks to a tenant before any tenant
		// context exists, so it is unique-indexed and never the raw tenant_id.
		// Resolving by token still requires the tenant's secret to verify the HMAC.
		field.String("payment_webhook_token").
			MaxLen(64).
			Optional().
			Nillable().
			Unique(),
		// Plaintext: the instance name is part of the API URL, not a secret.
		field.String("payrexx_instance").
			MaxLen(100).
			Optional().
			Nillable(),
		field.Text("payrexx_api_secret_enc").
			Optional().
			Nillable(),
		field.Text("payrexx_webhook_secret_enc").
			Optional().
			Nillable(),
		field.Time("payrexx_configured_at").
			Optional().
			Nillable(),
		// Last inbound webhook whose HMAC verified — proof the stored webhook
		// secret matches what Payrexx signs with.
		field.Time("payrexx_last_webhook_at").
			Optional().
			Nillable(),
		field.Text("plunk_api_key_enc").
			Optional().
			Nillable(),
		field.String("plunk_from_name").
			MaxLen(100).
			Optional().
			Nillable(),
		field.String("plunk_from_email").
			MaxLen(255).
			Optional().
			Nillable(),
		field.String("plunk_reply_to").
			MaxLen(255).
			Optional().
			Nillable(),
		field.Time("plunk_configured_at").
			Optional().
			Nillable(),
		// AES-256-GCM ciphertext (base64) of the 32-byte Ed25519 private seed;
		// the plaintext seed is never stored.
		field.Text("ed25519_private_seed_enc").
			Optional().
			Nillable(),
		// base64url(no-pad) Ed25519 public key; public by design, served at /v1/qr-config.
		field.Text("ed25519_public_key").
			Optional().
			Nillable(),
		// Set when the vendor finishes (or dismisses) the onboarding wizard. Per-step
		// progress is derived from real data (brand/payments/domain); this single
		// flag only records that the vendor is done, so the hub stops nudging.
		field.Time("onboarding_completed_at").
			Optional().
			Nillable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}
