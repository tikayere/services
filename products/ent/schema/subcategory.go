package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// SubCategory holds the schema definition for the SubCategory entity.
type SubCategory struct {
	ent.Schema
}

// Fields of the SubCategory.
func (SubCategory) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name").NotEmpty().Unique(),
		field.Text("description").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the SubCategory.
func (SubCategory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("category", Category.Type).Unique().Required(),
		edge.From("products", Product.Type).Ref("subcategory"),
	}
}
