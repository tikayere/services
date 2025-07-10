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

// Cart holds the schema definition for the Cart entity.
type Cart struct {
	ent.Schema
}

// Fields of the Cart.
func (Cart) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}).Comment("Reference to the user who owns the cart"),
		field.Time("expires_at").Default(func() time.Time {
			return time.Now().Add(7 * 24 * time.Hour)
		}).Comment("Cart expiration time"),
		field.Time("last_activity_at").Default(time.Now).UpdateDefault(time.Now).Comment("Last time cart was modified"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("deleted_at").Optional().Nillable().Comment("soft delete timestamp"),
		field.Int("version").Default(1).Comment("Optimistic lock version"),
	}
}

// Edges of the Cart.
func (Cart) Edges() []ent.Edge {
	return []ent.Edge{
		// A cart has many cart items
		edge.To("cart_items", CartItem.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

// Annotations of the cart
func (Cart) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "carts",
		},
	}
}
