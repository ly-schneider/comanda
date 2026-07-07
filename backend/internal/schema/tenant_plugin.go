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

type TenantPlugin struct {
	ent.Schema
}

func (TenantPlugin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tenant_plugin"},
	}
}

func (TenantPlugin) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (TenantPlugin) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("plugin_key").
			MaxLen(64).
			NotEmpty(),
		field.Bool("enabled").
			Default(false),
		field.JSON("config", json.RawMessage{}).
			Optional(),
		field.Bytes("secrets_enc").
			Optional().
			Nillable(),
		field.Time("secrets_configured_at").
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

func (TenantPlugin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "plugin_key").Unique(),
	}
}
