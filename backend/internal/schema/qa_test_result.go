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

// QaTestResult is one test's outcome within a run, keyed by the web
// checklist's stable test key; a null status means "not yet tested".
type QaTestResult struct {
	ent.Schema
}

func (QaTestResult) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "qa_test_result"},
	}
}

func (QaTestResult) Fields() []ent.Field {
	return []ent.Field{
		nanoidPK(),
		field.String("run_id").MaxLen(12),
		field.String("test_key").
			MaxLen(120).
			NotEmpty(),
		field.Enum("status").
			Values("success", "error").
			Optional().
			Nillable().
			StorageKey("status"),
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

func (QaTestResult) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("run", QaTestRun.Type).
			Ref("results").
			Field("run_id").
			Unique().
			Required(),
	}
}

func (QaTestResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_id", "test_key").Unique(),
	}
}
