package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Voucher struct {
	ent.Schema
}

func (Voucher) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "voucher"},
	}
}

func (Voucher) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (Voucher) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.Enum("kind").
			Values("pass", "entitlement").
			Default("pass").
			Immutable(),
		field.String("label").
			MaxLen(80).
			NotEmpty().
			Immutable(),
		field.Time("valid_from").
			Immutable(),
		field.Time("valid_until").
			Immutable(),
		field.Time("revoked_at").
			Optional().
			Nillable(),
		// Snapshot of the redeemable categories ([{id,name}]) as they were at
		// issue time, so a later rename never mutates a printed voucher.
		field.JSON("categories", json.RawMessage{}).
			Default(func() json.RawMessage { return json.RawMessage("[]") }).
			Immutable(),
		field.JSON("products", json.RawMessage{}).
			Optional().
			Immutable(),
		field.String("batch_id").
			MaxLen(12).
			Optional().
			Immutable(),
		field.Text("token").
			Immutable(),
		field.Time("redeemed_at").
			Optional().
			Nillable(),
		field.String("redeemed_order_id").
			MaxLen(12).
			Optional(),
		field.String("redeemed_device_id").
			MaxLen(12).
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (Voucher) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "batch_id"),
	}
}
