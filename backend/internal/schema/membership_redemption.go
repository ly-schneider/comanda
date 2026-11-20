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

// MembershipRedemption records what the membership plugin booked with the
// tenant's provider for one order. The provider owns the member's budget; this
// row only remembers what to give back when the order comes back.
type MembershipRedemption struct {
	ent.Schema
}

func (MembershipRedemption) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "membership_redemption"},
	}
}

func (MembershipRedemption) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (MembershipRedemption) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		// The provider's own handle for the member, opaque to Comanda.
		field.String("member_ref").
			MaxLen(100).
			NotEmpty(),
		field.String("member_name").
			MaxLen(100).
			Optional(),
		field.String("order_id").MaxLen(12),
		field.Int("quantity").
			Default(1),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		// Set once the provider confirmed the release; until then a refunded
		// order is still charged to the member and the next refund retries.
		field.Time("released_at").
			Optional().
			Nillable(),
	}
}

// Looked up by order: a refund asks what an order booked.
func (MembershipRedemption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id"),
	}
}

func (MembershipRedemption) Edges() []ent.Edge {
	// Unidirectional: the FK lives here and Order keeps no back-reference, so
	// the core stays free of the plugin.
	return []ent.Edge{
		edge.To("order", Order.Type).
			Field("order_id").
			Unique().
			Required(),
	}
}
