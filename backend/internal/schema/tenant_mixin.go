package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

// TenantMixin adds the tenant_id column, its index, and a one-directional
// foreign key to the tenant root table to every tenant-scoped entity.
//
// Keeping the FK in the Ent schema (rather than hand-writing it in a
// migration) ensures Atlas migrate-diff stays drift-free. The column is the
// anchor for the Postgres row-level-security policies installed in the
// tenant-and-RLS migration; RLS — not this FK — is the isolation boundary.
//
// It lives in the schema package (not a sub-package) to avoid an import cycle:
// each tenant-scoped schema imports this mixin, and the mixin references
// Tenant.Type.
type TenantMixin struct {
	mixin.Schema
}

func (TenantMixin) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("tenant_id", uuid.UUID{}).
			Immutable(),
	}
}

func (TenantMixin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tenant", Tenant.Type).
			Field("tenant_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (TenantMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
	}
}
