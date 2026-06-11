package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"backend/internal/id"
)

// nanoidPK is the shared primary-key field: a 12-char nanoid generated
// app-side. No DB-side default — the platform's managed Postgres (PG17)
// has no uuidv7(), and IDs come from internal/id only.
func nanoidPK() ent.Field {
	return field.String("id").
		MaxLen(12).
		NotEmpty().
		DefaultFunc(id.New).
		Immutable()
}
