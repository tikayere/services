package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go-micro.dev/v5/logger"

	"orders/ent"
	"orders/ent/order"
	pb "orders/proto"
)

// OrderService implements the OrderServiceServer interface
type OrderService struct {
	EntClient *ent.Client
}

// CreateOrder handles the creation of a new order
func (h *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest, rsp *pb.CreateOrderResponse) error {
	logger.Infof("Received CreateOrder request for user_id: %s", req.UserId)

	// Calculate total amount
	var totalAmount float64
	for _, item := range req.OrderItems {
		totalAmount += float64(item.Quantity) * item.UnitPrice
	}

	// Start a transaction
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		logger.Errorf("Failed to start transaction: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Create order
	o, err := tx.Order.Create().
		SetUserID(uuid.MustParse(req.UserId)).
		SetTotalAmount(totalAmount).
		Save(ctx)
	if ent.IsConstraintError(err) {
		logger.Errorf("Constraint violation: %v", err)
		return fmt.Errorf("constraint violation: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to create order: %v", err)
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Create order items
	for _, item := range req.OrderItems {
		_, err = tx.OrderItem.Create().
			SetOrderID(o.ID).
			SetProductID(uuid.MustParse(item.ProductId)).
			SetQuantity(int(item.Quantity)).
			SetUnitPrice(item.UnitPrice).
			Save(ctx)
		if err != nil {
			logger.Errorf("Failed to create order item for product %s: %v", item.ProductId, err)
			return fmt.Errorf("failed to create order item: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		logger.Errorf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch order with items
	oWithItems, err := h.EntClient.Order.Query().
		Where(order.ID(o.ID)).
		WithOrderItems().
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch order with items: %v", err)
		return fmt.Errorf("failed to fetch order: %w", err)
	}

	rsp.Order = toProtoOrder(oWithItems)
	logger.Infof("Order created successfully: %s", o.ID)
	return nil
}

// GetOrder handles fetching an order by ID
func (h *OrderService) GetOrder(ctx context.Context, req *pb.GetOrderRequest, rsp *pb.GetOrderResponse) error {
	logger.Infof("Received GetOrder request for ID: %s", req.Id)

	o, err := h.EntClient.Order.Query().
		Where(order.ID(uuid.MustParse(req.Id))).
		WithOrderItems().
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Order not found: %s", req.Id)
		return fmt.Errorf("order not found")
	}
	if err != nil {
		logger.Errorf("Failed to get order: %v", err)
		return fmt.Errorf("failed to get order: %w", err)
	}

	rsp.Order = toProtoOrder(o)
	logger.Infof("Order fetched successfully: %s", o.ID)
	return nil
}

// UpdateOrderStatus handles updating an order's status
func (h *OrderService) UpdateOrderStatus(ctx context.Context, req *pb.UpdateOrderStatusRequest, rsp *pb.UpdateOrderStatusResponse) error {
	logger.Infof("Received UpdateOrderStatus request for ID: %s, status: %s", req.Id, req.Status)

	// Validate status
	validStatuses := map[string]bool{
		"pending":    true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
		"cancelled":  true,
	}
	if !validStatuses[req.Status] {
		logger.Infof("Invalid status: %s", req.Status)
		return fmt.Errorf("invalid status: %s", req.Status)
	}

	o, err := h.EntClient.Order.UpdateOneID(uuid.MustParse(req.Id)).
		SetStatus(order.Status(req.Status)).
		Save(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Order not found for update: %s", req.Id)
		return fmt.Errorf("order not found")
	}
	if err != nil {
		logger.Errorf("Failed to update order status: %v", err)
		return fmt.Errorf("failed to update order status: %w", err)
	}

	// Fetch order with items
	oWithItems, err := h.EntClient.Order.Query().
		Where(order.ID(o.ID)).
		WithOrderItems().
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch order with items: %v", err)
		return fmt.Errorf("failed to fetch order: %w", err)
	}

	rsp.Order = toProtoOrder(oWithItems)
	logger.Infof("Order status updated successfully: %s", o.ID)
	return nil
}

// ListOrders handles listing all orders with optional filtering and pagination
func (h *OrderService) ListOrders(ctx context.Context, req *pb.ListOrdersRequest, rsp *pb.ListOrdersResponse) error {
	logger.Infof("Received ListOrders request (limit: %d, offset: %d, user_id: %s)", req.Limit, req.Offset, req.UserId)

	query := h.EntClient.Order.Query().WithOrderItems()

	if req.UserId != "" {
		query.Where(order.UserID(uuid.MustParse(req.UserId)))
	}

	if req.Limit > 0 {
		// Ensure limit does not exceed int max
		if req.Limit > int32(uint(0)>>1) {
			logger.Infof("Limit %d exceeds maximum allowed value, capping at %d", req.Limit, int32(uint(0)>>1))
			req.Limit = int32(uint(0) >> 1)
		}
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		// Ensure offset does not exceed int max
		if req.Offset > int32(uint(0)>>1) {
			logger.Infof("Offset %d exceeds maximum allowed value, capping at %d", req.Offset, int32(uint(0)>>1))
			req.Offset = int32(uint(0) >> 1)
		}
		query.Offset(int(req.Offset))
	}

	orders, err := query.All(ctx)
	if err != nil {
		logger.Errorf("Failed to list orders: %v", err)
		return fmt.Errorf("failed to list orders: %w", err)
	}

	q := h.EntClient.Order.Query()
	if req.UserId != "" {
		userID, err := uuid.Parse(req.UserId)
		if err != nil {
			return fmt.Errorf("invalid user id: %v", err)
		}
		q = q.Where(order.UserID(userID))
	}
	total, err := q.Count(ctx)
	if err != nil {
		logger.Errorf("Failed to count orders: %v", err)
		return fmt.Errorf("failed to count orders: %w", err)
	}

	protoOrders := make([]*pb.Order, len(orders))
	for i, o := range orders {
		protoOrders[i] = toProtoOrder(o)
	}

	rsp.Orders = protoOrders
	rsp.Total = int32(total)
	logger.Infof("Listed %d orders (total: %d)", len(protoOrders), total)
	return nil
}

// SearchOrders searches orders by user_id and/or status
func (h *OrderService) SearchOrders(ctx context.Context, req *pb.SearchOrdersRequest, rsp *pb.SearchOrdersResponse) error {
	logger.Infof("Received SearchOrders request (user_id: %s, status: %s, limit: %d, offset: %d)", req.UserId, req.Status, req.Limit, req.Offset)

	query := h.EntClient.Order.Query().WithOrderItems()

	if req.UserId != "" {
		query.Where(order.UserID(uuid.MustParse(req.UserId)))
	}
	if req.Status != "" {
		query.Where(order.StatusEQ(order.Status(req.Status)))
	}

	if req.Limit > 0 {
		// Ensure limit does not exceed int max
		if req.Limit > int32(uint(0)>>1) {
			logger.Infof("Limit %d exceeds maximum allowed value, capping at %d", req.Limit, int32(uint(0)>>1))
			req.Limit = int32(uint(0) >> 1)
		}
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		// Ensure offset does not exceed int max
		if req.Offset > int32(uint(0)>>1) {
			logger.Infof("Offset %d exceeds maximum allowed value, capping at %d", req.Offset, int32(uint(0)>>1))
			req.Offset = int32(uint(0) >> 1)
		}
		query.Offset(int(req.Offset))
	}

	orders, err := query.All(ctx)
	if err != nil {
		logger.Errorf("Failed to search orders: %v", err)
		return fmt.Errorf("failed to search orders: %w", err)
	}

	q := h.EntClient.Order.Query()
	// user Id filter
	if req.UserId != "" {
		userID, err := uuid.Parse(req.UserId)
		if err != nil {
			return fmt.Errorf("invalid user ID: %v", err)
		}
		q = q.Where(order.UserID(userID))
	}

	// Status filter
	if req.Status != "" {
		status := order.Status(req.Status)
		q = q.Where(order.StatusEQ(status))
	}

	total, err := q.Count(ctx)

	if err != nil {
		logger.Errorf("Failed to count orders for search: %v", err)
		return fmt.Errorf("failed to count orders for search: %w", err)
	}

	protoOrders := make([]*pb.Order, len(orders))
	for i, o := range orders {
		protoOrders[i] = toProtoOrder(o)
	}

	rsp.Orders = protoOrders
	rsp.Total = int32(total)
	logger.Infof("Found %d orders (total: %d)", len(protoOrders), total)
	return nil
}

// toProtoOrder converts an Entgo Order entity to a Protobuf Order message
func toProtoOrder(o *ent.Order) *pb.Order {
	if o == nil {
		return nil
	}
	protoOrder := &pb.Order{
		Id:          o.ID.String(),
		UserId:      o.UserID.String(),
		TotalAmount: o.TotalAmount,
		Status:      o.Status.String(),
		CreatedAt:   o.CreatedAt.Unix(),
		UpdatedAt:   o.UpdatedAt.Unix(),
	}
	if o.Edges.OrderItems != nil {
		protoOrder.OrderItems = make([]*pb.OrderItem, len(o.Edges.OrderItems))
		for i, item := range o.Edges.OrderItems {
			protoOrder.OrderItems[i] = &pb.OrderItem{
				Id:        item.ID.String(),
				ProductId: item.ProductID.String(),
				Quantity:  int32(item.Quantity),
				UnitPrice: item.UnitPrice,
				CreatedAt: item.CreatedAt.Unix(),
				UpdatedAt: item.UpdatedAt.Unix(),
				OrderId:   o.ID.String(),
			}
		}
	}
	return protoOrder
}
