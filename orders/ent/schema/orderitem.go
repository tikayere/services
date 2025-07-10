package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// OrderItem holds the schema definition for the OrderItem entity.
type OrderItem struct {
	ent.Schema
}

// Fields of the OrderItem.
func (OrderItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("product_id", uuid.UUID{}).Comment("Reference to the product"),
		field.Int("quantity").Positive(),
		field.Float("unit_price").Positive(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the OrderItem.
func (OrderItem) Edges() []ent.Edge {
	return []ent.Edge{
		// An order item belongs to one order
		edge.To("order", Order.Type).Unique().Required(),
	}
}
