package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// QaTestRun is a named manual-QA pass; platform-plane (no tenant_id, RLS
// gates on app_is_platform_admin()).
type QaTestRun struct {
	ent.Schema
}

func (QaTestRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "qa_test_run"},
	}
}

func (QaTestRun) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("name").
			MaxLen(120).
			NotEmpty(),
		field.Text("note").
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

func (QaTestRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("results", QaTestResult.Type),
	}
}
