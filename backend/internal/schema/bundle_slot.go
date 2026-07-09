package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type BundleSlot struct {
	ent.Schema
}

func (BundleSlot) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_slot"},
	}
}

func (BundleSlot) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (BundleSlot) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("bundle_product_id").MaxLen(12),
		field.String("name").
			MaxLen(20).
			NotEmpty(),
		field.Int("sequence").
			Default(0),
	}
}

func (BundleSlot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("bundle_product", Product.Type).
			Ref("bundle_slots").
			Field("bundle_product_id").
			Unique().
			Required(),
		edge.To("option_products", Product.Type).
			Through("options", BundleSlotOption.Type),
		edge.To("order_lines", OrderLine.Type),
	}
}
