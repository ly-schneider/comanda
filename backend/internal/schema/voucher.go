package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
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
			Immutable(),
		// Signed 0x03 display token (see internal/qrsign); persisted at creation
		// so the PDF is a pure re-read.
		field.Text("token").
			Immutable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}
