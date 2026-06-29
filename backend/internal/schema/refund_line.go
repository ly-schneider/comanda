package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type RefundLine struct {
	ent.Schema
}

func (RefundLine) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "refund_line"},
	}
}

func (RefundLine) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (RefundLine) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("refund_id").MaxLen(12),
		field.String("order_line_id").MaxLen(12),
		field.Int("quantity").
			Positive(),
		field.Int64("amount_cents"),
	}
}

func (RefundLine) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("refund", Refund.Type).
			Ref("refund_lines").
			Field("refund_id").
			Unique().
			Required(),
		edge.From("order_line", OrderLine.Type).
			Ref("refund_lines").
			Field("order_line_id").
			Unique().
			Required(),
	}
}
