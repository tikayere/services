package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go-micro.dev/v5/logger"

	"orders/ent"
	"orders/ent/order"
	"orders/ent/orderitem"
	pb "orders/proto"
)

// AdminService implements the AdminServiceServer interface
type AdminService struct {
	EntClient *ent.Client
}

// ForceDeleteOrder handles the forced deletion of an order (admin privilege)
func (h *AdminService) ForceDeleteOrder(ctx context.Context, req *pb.ForceDeleteOrderRequest, rsp *pb.ForceDeleteOrderResponse) error {
	logger.Infof("Received ForceDeleteOrder request for ID: %s (Admin operation)", req.Id)

	// Start a transaction to ensure atomicity
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		logger.Errorf("Failed to start transaction for force delete: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete order items first due to foreign key constraints
	_, err = tx.OrderItem.Delete().
		Where(orderitem.HasOrderWith(order.ID(uuid.MustParse(req.Id)))).
		Exec(ctx)
	if err != nil {
		logger.Errorf("Failed to delete order items for order %s: %v", req.Id, err)
		return fmt.Errorf("failed to delete order items: %w", err)
	}

	// Delete order
	err = tx.Order.DeleteOneID(uuid.MustParse(req.Id)).Exec(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Order not found for deletion: %s", req.Id)
		rsp.Success = false
		return fmt.Errorf("order not found for deletion: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to force delete order: %v", err)
		rsp.Success = false
		return fmt.Errorf("failed to force delete order: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		logger.Errorf("Failed to commit transaction for force delete: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	rsp.Id = req.Id
	rsp.Success = true
	logger.Infof("Order force deleted successfully: %s", req.Id)
	return nil
}

// BulkCreateOrders handles streaming creation of multiple orders
func (h *AdminService) BulkCreateOrders(ctx context.Context, stream pb.AdminService_BulkCreateOrdersStream) error {
	logger.Infof("Received BulkCreateOrders stream request (Admin operation)")
	var createdOrders []*pb.Order
	var totalCreated int32

	for {
		req := &pb.CreateOrderRequest{}
		err := stream.RecvMsg(req)
		if err != nil {
			if err.Error() == "EOF" { // go-micro uses EOF for end of stream
				break
			}
			logger.Errorf("Error receiving from BulkCreateOrders stream: %v", err)
			return fmt.Errorf("error receiving order data: %w", err)
		}

		logger.Infof("Bulk creating order for user_id: %s", req.UserId)

		// Calculate total amount
		var totalAmount float64
		for _, item := range req.OrderItems {
			totalAmount += float64(item.Quantity) * item.UnitPrice
		}

		// Start a transaction
		tx, err := h.EntClient.Tx(ctx)
		if err != nil {
			logger.Errorf("BulkCreateOrders: Failed to start transaction for user %s: %v", req.UserId, err)
			continue
		}

		// Create order
		o, err := tx.Order.Create().
			SetUserID(uuid.MustParse(req.UserId)).
			SetTotalAmount(totalAmount).
			Save(ctx)
		if ent.IsConstraintError(err) {
			logger.Errorf("BulkCreateOrders: Constraint violation for user %s: %v", req.UserId, err)
			tx.Rollback()
			continue
		}
		if err != nil {
			logger.Errorf("BulkCreateOrders: Failed to create order for user %s: %v", req.UserId, err)
			tx.Rollback()
			continue
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
				logger.Errorf("BulkCreateOrders: Failed to create order item for product %s: %v", item.ProductId, err)
				tx.Rollback()
				continue
			}
		}

		if err = tx.Commit(); err != nil {
			logger.Errorf("BulkCreateOrders: Failed to commit transaction for order %s: %v", o.ID, err)
			continue
		}

		// Fetch order with items
		oWithItems, err := h.EntClient.Order.Query().
			Where(order.ID(o.ID)).
			WithOrderItems().
			Only(ctx)
		if err != nil {
			logger.Errorf("BulkCreateOrders: Failed to fetch order with items %s: %v", o.ID, err)
			continue
		}

		createdOrders = append(createdOrders, toProtoOrder(oWithItems))
		totalCreated++
	}

	// Send the final response
	err := stream.SendMsg(&pb.BulkCreateOrdersResponse{
		Orders: createdOrders,
		Total:  totalCreated,
	})
	if err != nil {
		logger.Errorf("Error sending BulkCreateOrders response: %v", err)
		return fmt.Errorf("failed to send response: %w", err)
	}

	logger.Infof("BulkCreateOrders: Successfully created %d orders.", totalCreated)
	return nil
}

// ExportOrders streams all orders, optionally filtered and paginated
func (h *AdminService) ExportOrders(ctx context.Context, req *pb.ExportOrdersRequest, stream pb.AdminService_ExportOrdersStream) error {
	logger.Infof("Received ExportOrders stream request (limit: %d, offset: %d, user_id: %s, status: %s)", req.Limit, req.Offset, req.UserId, req.Status)

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
		logger.Errorf("Failed to retrieve orders for export: %v", err)
		return fmt.Errorf("failed to retrieve orders for export: %w", err)
	}

	for _, o := range orders {
		protoOrder := toProtoOrder(o)
		if err := stream.Send(protoOrder); err != nil {
			logger.Errorf("Error sending order %s during export: %v", o.ID, err)
			return fmt.Errorf("failed to stream order: %w", err)
		}
	}

	logger.Infof("Successfully exported %d orders.", len(orders))
	return nil
}
