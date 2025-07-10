package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// CartItem holds the schema definition for the CartItem entity.
type CartItem struct {
	ent.Schema
}

// Fields of the CartItem.
func (CartItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("product_id", uuid.UUID{}).Comment("Reference to the product"),
		field.Int("quantity").Positive(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the CartItem.
func (CartItem) Edges() []ent.Edge {
	return []ent.Edge{
		// A cart item belongs to one cart
		edge.To("cart", Cart.Type).Unique().Required(),
	}
}

// Annotations of the CartItem.
func (CartItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "cart_items",
		},
	}
}
