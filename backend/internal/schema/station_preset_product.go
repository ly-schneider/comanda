package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type StationPresetProduct struct {
	ent.Schema
}

func (StationPresetProduct) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "station_preset_product"},
		field.ID("preset_id", "product_id"),
	}
}

func (StationPresetProduct) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (StationPresetProduct) Fields() []ent.Field {
	return []ent.Field{
		field.String("preset_id").MaxLen(12),
		field.String("product_id").MaxLen(12),
	}
}

func (StationPresetProduct) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("preset", StationEnrollPreset.Type).
			Field("preset_id").
			Unique().
			Required(),
		edge.To("product", Product.Type).
			Field("product_id").
			Unique().
			Required(),
	}
}
