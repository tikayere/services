package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Profile holds the schema definition for the Profile entity.
type Profile struct {
	ent.Schema
}

// Fields of the Profile.
func (Profile) Fields() []ent.Field {
	return []ent.Field{
		field.String("first_name").Optional().Nillable(),
		field.String("last_name").Optional().Nillable(),
		field.Time("date_of_birth").Optional().Nillable(),
		field.String("address").Optional().Nillable(),
		field.String("phone_number").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the Profile.
func (Profile) Edges() []ent.Edge {
	return []ent.Edge{
		// A profile belongs to exactly one user (one-to-one relationship)
		// and the user field is unique and required
		edge.From("user", User.Type).
			Ref("profile").Unique().Required(),
	}
}
