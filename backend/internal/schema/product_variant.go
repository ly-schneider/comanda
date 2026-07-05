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

type ProductVariant struct {
	ent.Schema
}

func (ProductVariant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "product_variant"},
	}
}

func (ProductVariant) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (ProductVariant) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("product_id").MaxLen(12),
		field.String("value").
			MaxLen(20).
			NotEmpty(),
		field.Int64("price_adjustment_cents").
			Default(0),
		field.Bool("is_active").
			Default(true),
		field.Int("sequence").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (ProductVariant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("product", Product.Type).
			Ref("variants").
			Field("product_id").
			Unique().
			Required(),
		edge.To("order_lines", OrderLine.Type),
		edge.To("inventory_ledger_entries", InventoryLedger.Type),
	}
}

func (ProductVariant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "product_id", "value").
			Unique(),
		index.Fields("product_id"),
	}
}
