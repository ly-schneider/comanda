package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Category struct {
	ent.Schema
}

func (Category) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "category"},
	}
}

func (Category) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (Category) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("name").
			MaxLen(20).
			NotEmpty(),
		field.Bool("is_active").
			Default(true),
		field.Int("position").
			Default(0),
		field.String("parent_id").MaxLen(12).
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

func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("products", Product.Type),
		edge.To("children", Category.Type),
		edge.From("parent", Category.Type).
			Ref("children").
			Field("parent_id").
			Unique().
			Annotations(entsql.Annotation{OnDelete: entsql.Restrict}),
	}
}

func (Category) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_id"),
	}
}
