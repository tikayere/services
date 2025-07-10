package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("email").Unique().Immutable().Comment("Email should be unique and not change after creation"),
		field.String("username").Unique().NotEmpty(),
		field.String("password_hash").NotEmpty(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Bool("is_active").Default(true),
		field.Bool("email_verified").Default(false),
		field.String("verification_token").Optional().Nillable(),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		// A user has one profile (one-to-one relationship)
		edge.To("profile", Profile.Type).Unique(),
	}
}
