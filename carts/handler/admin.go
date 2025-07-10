package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go-micro.dev/v5/logger"

	"carts/ent"
	"carts/ent/cart"
	"carts/ent/cartitem"
	pb "carts/proto"
)

// AdminService implements the AdminServiceServer interface
type AdminService struct {
	EntClient *ent.Client
}

// ListCarts lists all carts with optional filtering and pagination
func (h *AdminService) ListCarts(ctx context.Context, req *pb.ListCartsRequest, rsp *pb.ListCartsResponse) error {
	logger.Infof("Received ListCarts request (limit: %d, offset: %d, user_id: %s, include_deleted: %v)", req.Limit, req.Offset, req.UserId, req.IncludeDeleted)

	query := h.EntClient.Cart.Query().WithCartItems()

	if req.UserId != "" {
		query.Where(cart.UserID(uuid.MustParse(req.UserId)))
	}
	if !req.IncludeDeleted {
		query.Where(cart.DeletedAtIsNil())
	}

	if req.Limit > 0 {
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		query.Offset(int(req.Offset))
	}

	carts, err := query.All(ctx)
	if err != nil {
		logger.Errorf("Failed to list carts: %v", err)
		return fmt.Errorf("failed to list carts: %w", err)
	}
	q := h.EntClient.Cart.Query()
	if req.UserId != "" {
		userId, err := uuid.Parse(req.UserId)
		if err != nil {
			fmt.Errorf("invalid user Id, %v", err)
		}
		q = q.Where(cart.UserID(userId))
	}
	total, err := q.Count(ctx)
	if err != nil {
		logger.Errorf("Failed to count carts: %v", err)
		return fmt.Errorf("failed to count carts: %w", err)
	}

	protoCarts := make([]*pb.Cart, len(carts))
	for i, c := range carts {
		protoCarts[i] = toProtoCart(c)
	}

	rsp.Carts = protoCarts
	rsp.Total = int32(total)
	logger.Infof("Listed %d carts (total: %d)", len(protoCarts), total)
	return nil
}

// ForceDeleteCart permanently deletes a cart and its items (admin privilege)
func (h *AdminService) ForceDeleteCart(ctx context.Context, req *pb.ForceDeleteCartRequest, rsp *pb.ForceDeleteCartResponse) error {
	logger.Infof("Received ForceDeleteCart request for ID: %s (Admin operation)", req.Id)

	// Start a transaction
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		logger.Errorf("Failed to start transaction for force delete: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete cart items first due to foreign key constraints
	_, err = tx.CartItem.Delete().
		Where(cartitem.HasCartWith(cart.ID(uuid.MustParse(req.Id)))).
		Exec(ctx)
	if err != nil {
		logger.Errorf("Failed to delete cart items for cart %s: %v", req.Id, err)
		return fmt.Errorf("failed to delete cart items: %w", err)
	}

	// Delete cart
	err = tx.Cart.DeleteOneID(uuid.MustParse(req.Id)).Exec(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found for deletion: %s", req.Id)
		rsp.Success = false
		return fmt.Errorf("cart not found for deletion: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to force delete cart: %v", err)
		rsp.Success = false
		return fmt.Errorf("failed to force delete cart: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		logger.Errorf("Failed to commit transaction for force delete: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	rsp.Id = req.Id
	rsp.Success = true
	logger.Infof("Cart force deleted successfully: %s", req.Id)
	return nil
}

// RestoreCart restores a soft-deleted cart
func (h *AdminService) RestoreCart(ctx context.Context, req *pb.RestoreCartRequest, rsp *pb.RestoreCartResponse) error {
	logger.Infof("Received RestoreCart request for ID: %s (Admin operation)", req.Id)

	c, err := h.EntClient.Cart.UpdateOneID(uuid.MustParse(req.Id)).
		ClearDeletedAt().
		AddVersion(1).
		Save(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found for restoration: %s", req.Id)
		return fmt.Errorf("cart not found for restoration")
	}
	if err != nil {
		logger.Errorf("Failed to restore cart: %v", err)
		return fmt.Errorf("failed to restore cart: %w", err)
	}

	rsp.Cart = toProtoCart(c)
	logger.Infof("Cart restored successfully: %s", req.Id)
	return nil
}

// ExportCarts streams all carts, optionally filtered and paginated
func (h *AdminService) ExportCarts(ctx context.Context, req *pb.ExportCartsRequest, stream pb.AdminService_ExportCartsStream) error {
	logger.Infof("Received ExportCarts stream request (limit: %d, offset: %d, user_id: %s, include_deleted: %v)", req.Limit, req.Offset, req.UserId, req.IncludeDeleted)

	query := h.EntClient.Cart.Query().WithCartItems()

	if req.UserId != "" {
		query.Where(cart.UserID(uuid.MustParse(req.UserId)))
	}
	if !req.IncludeDeleted {
		query.Where(cart.DeletedAtIsNil())
	}

	if req.Limit > 0 {
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		query.Offset(int(req.Offset))
	}

	carts, err := query.All(ctx)
	if err != nil {
		logger.Errorf("Failed to retrieve carts for export: %v", err)
		return fmt.Errorf("failed to retrieve carts for export: %w", err)
	}

	for _, c := range carts {
		protoCart := toProtoCart(c)
		if err := stream.Send(protoCart); err != nil {
			logger.Errorf("Error sending cart %s during export: %v", c.ID, err)
			return fmt.Errorf("failed to stream cart: %w", err)
		}
	}

	logger.Infof("Successfully exported %d carts.", len(carts))
	return nil
}
