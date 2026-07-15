package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type StationEnrollPreset struct {
	ent.Schema
}

func (StationEnrollPreset) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "station_enroll_preset"},
	}
}

func (StationEnrollPreset) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (StationEnrollPreset) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("name").
			MaxLen(60).
			NotEmpty(),
		field.Bytes("token_enc"),
		field.String("issued_by").
			MaxLen(64).
			NotEmpty(),
		field.Time("disabled_at").
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

func (StationEnrollPreset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("products", Product.Type).
			Through("preset_products", StationPresetProduct.Type),
	}
}
