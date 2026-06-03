package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TenantDomain maps a public hostname to a tenant. The host-based resolver
// reads this table via the privileged client (before app.tenant_id is set) to
// turn the request Host into a tenant. It is still tenant-scoped (embeds
// TenantMixin) so the app role and the Ent guards only ever see their own rows.
type TenantDomain struct {
	ent.Schema
}

func (TenantDomain) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tenant_domain"},
	}
}

func (TenantDomain) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TenantMixin{},
	}
}

func (TenantDomain) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuidV7).
			Immutable(),
		// Lowercase-only (enforced by a DB CHECK); unique via the index below.
		field.String("hostname").
			NotEmpty(),
		field.Bool("is_primary").
			Default(false),
		field.Time("verified_at").
			Optional().
			Nillable(),
		field.Enum("cert_status").
			Values("none", "pending", "active", "failed").
			Default("none").
			StorageKey("cert_status"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (TenantDomain) Edges() []ent.Edge {
	return nil
}

func (TenantDomain) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("hostname").Unique(),
	}
}
