package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type StationEnrollToken struct {
	ent.Schema
}

func (StationEnrollToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "station_enroll_token"},
	}
}

func (StationEnrollToken) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (StationEnrollToken) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
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
