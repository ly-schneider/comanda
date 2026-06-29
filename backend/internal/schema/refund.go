package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Refund struct {
	ent.Schema
}

func (Refund) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "refund"},
	}
}

func (Refund) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (Refund) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("order_id").MaxLen(12),
		field.Int64("amount_cents"),
		field.Enum("kind").
			Values("line", "arbitrary").
			StorageKey("kind"),
		field.Enum("method").
			Values("cash", "twint").
			StorageKey("method"),
		field.Text("reason").
			Optional().
			Nillable(),
		field.Enum("actor_type").
			Values("operator", "admin").
			StorageKey("actor_type"),
		field.String("actor_id"),
		field.String("device_id").MaxLen(12).
			Optional().
			Nillable(),
		field.Int("payrexx_transaction_id").
			Optional().
			Nillable(),
		field.Enum("status").
			Values("pending", "succeeded", "failed").
			StorageKey("status"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (Refund) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("refunds").
			Field("order_id").
			Unique().
			Required(),
		edge.To("refund_lines", RefundLine.Type),
	}
}
