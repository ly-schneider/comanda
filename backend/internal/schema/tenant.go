package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Tenant struct {
	ent.Schema
}

func (Tenant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tenant"},
	}
}

func (Tenant) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("slug").
			MaxLen(63).
			NotEmpty().
			Unique(),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		// Suffix of the tenant's Android applicationId. Immutable: a tablet keys its
		// install off the applicationId, so a change makes the next build a
		// different app — no upgrade path, re-pairing, two icons.
		field.String("android_package_id").
			MaxLen(66).
			Optional().
			Nillable().
			Immutable().
			Unique(),
		// 1:1 link to the tenant's Better Auth organization. Its FK, unique index,
		// and org-keyed RLS live in the migration (organization is not an Ent type).
		field.String("organization_id").
			Optional().
			Nillable().
			Immutable(),
		field.Enum("status").
			Values("active", "suspended").
			Default("active").
			StorageKey("status"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Tenant) Edges() []ent.Edge {
	return nil
}

func (Tenant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug").Unique(),
	}
}
