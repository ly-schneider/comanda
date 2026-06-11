package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"backend/internal/id"
)

func nanoidPK() ent.Field {
	return field.String("id").
		MaxLen(12).
		NotEmpty().
		DefaultFunc(id.New).
		Immutable()
}
