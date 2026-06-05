package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
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
		field.Int("club100_max_redemptions").
			Default(2),
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
			Default("#f87778"),
		field.String("color_primary_foreground").
			MaxLen(9).
			Default("#fdfdfd"),
		field.String("color_background").
			MaxLen(9).
			Default("#e9e7e6"),
		field.String("color_foreground").
			MaxLen(9).
			Default("#000000"),
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
		field.Text("legal_text").
			Optional().
			Nillable(),
		field.String("currency").
			MaxLen(3).
			Default("CHF"),
		field.String("payment_provider").
			MaxLen(50).
			Default("payrexx"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Settings) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("club100_free_products", Product.Type).
			Through("club100_free_product_links", Club100FreeProduct.Type),
	}
}
