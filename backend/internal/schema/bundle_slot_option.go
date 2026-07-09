package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type BundleSlotOption struct {
	ent.Schema
}

func (BundleSlotOption) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_slot_option"},
		field.ID("bundle_slot_id", "option_product_id"),
	}
}

func (BundleSlotOption) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (BundleSlotOption) Fields() []ent.Field {
	return []ent.Field{
		field.String("bundle_slot_id").MaxLen(12),
		field.String("option_product_id").MaxLen(12),
	}
}

func (BundleSlotOption) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("bundle_slot", BundleSlot.Type).
			Field("bundle_slot_id").
			Unique().
			Required(),
		edge.To("option_product", Product.Type).
			Field("option_product_id").
			Unique().
			Required(),
	}
}
