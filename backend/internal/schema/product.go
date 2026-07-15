package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Product struct {
	ent.Schema
}

func (Product) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "product"},
	}
}

func (Product) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (Product) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("category_id").MaxLen(12),
		field.Enum("type").
			Values("simple", "bundle").
			Default("simple").
			StorageKey("type"),
		field.String("name").
			MaxLen(20).
			NotEmpty(),
		field.String("image").
			Optional().
			Nillable(),
		field.String("description").
			MaxLen(500).
			Optional().
			Nillable(),
		field.Int64("price_cents").
			Default(0),
		field.Int64("deposit_cents").
			Default(0),
		field.String("jeton_id").MaxLen(12).
			Optional().
			Nillable(),
		field.Bool("is_active").
			Default(true),
		field.String("variant_label").
			MaxLen(20).
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Product) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("category", Category.Type).
			Ref("products").
			Field("category_id").
			Unique().
			Required(),
		edge.From("jeton", Jeton.Type).
			Ref("products").
			Field("jeton_id").
			Unique(),
		edge.To("bundle_slots", BundleSlot.Type),
		edge.To("variants", ProductVariant.Type),
		edge.From("bundle_slot_options", BundleSlot.Type).
			Ref("option_products").
			Through("bundle_slot_option_links", BundleSlotOption.Type),
		edge.From("devices", Device.Type).
			Ref("products").
			Through("device_products", DeviceProduct.Type),
		edge.To("order_lines", OrderLine.Type),
		edge.To("inventory_ledger_entries", InventoryLedger.Type),
		edge.From("volunteer_campaigns", VolunteerCampaign.Type).
			Ref("products").
			Through("campaign_products", VolunteerCampaignProduct.Type),
		edge.From("station_presets", StationEnrollPreset.Type).
			Ref("products").
			Through("preset_products", StationPresetProduct.Type),
	}
}
