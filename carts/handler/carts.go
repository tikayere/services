package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go-micro.dev/v5/logger"

	"carts/ent"
	"carts/ent/cart"
	"carts/ent/cartitem"
	pb "carts/proto"
)

// CartService implements the CartServiceServer interface
type CartService struct {
	EntClient *ent.Client
}

// GetOrCreateCart gets an existing cart or creates a new one for the user
func (h *CartService) GetOrCreateCart(ctx context.Context, req *pb.GetOrCreateCartRequest, rsp *pb.GetOrCreateCartResponse) error {
	logger.Infof("Received GetOrCreateCart request for user_id: %s", req.UserId)

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		logger.Errorf("Invalid user_id format: %v", err)
		return fmt.Errorf("invalid user_id format: %w", err)
	}

	// Check for existing active cart
	c, err := h.EntClient.Cart.Query().
		Where(
			cart.UserID(userID),
			cart.DeletedAtIsNil(),
			cart.ExpiresAtGT(time.Now()),
		).
		WithCartItems().
		Only(ctx)
	if err == nil {
		// Update last activity
		c, err = h.EntClient.Cart.UpdateOneID(c.ID).
			SetLastActivityAt(time.Now()).
			SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
			Save(ctx)
		if err != nil {
			logger.Errorf("Failed to update cart activity: %v", err)
			return fmt.Errorf("failed to update cart: %w", err)
		}
		rsp.Cart = toProtoCart(c)
		logger.Infof("Retrieved existing cart: %s", c.ID)
		return nil
	}
	if !ent.IsNotFound(err) {
		logger.Errorf("Failed to query cart: %v", err)
		return fmt.Errorf("failed to query cart: %w", err)
	}

	// Create new cart
	c, err = h.EntClient.Cart.Create().
		SetUserID(userID).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		SetLastActivityAt(time.Now()).
		Save(ctx)
	if ent.IsConstraintError(err) {
		logger.Errorf("Constraint violation: %v", err)
		return fmt.Errorf("constraint violation: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to create cart: %v", err)
		return fmt.Errorf("failed to create cart: %w", err)
	}

	rsp.Cart = toProtoCart(c)
	logger.Infof("Created new cart: %s", c.ID)
	return nil
}

// GetCart fetches a cart by ID
func (h *CartService) GetCart(ctx context.Context, req *pb.GetCartRequest, rsp *pb.GetCartResponse) error {
	logger.Infof("Received GetCart request for ID: %s", req.Id)

	c, err := h.EntClient.Cart.Query().
		Where(
			cart.ID(uuid.MustParse(req.Id)),
			cart.DeletedAtIsNil(),
			cart.ExpiresAtGT(time.Now()),
		).
		WithCartItems().
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found or expired: %s", req.Id)
		return fmt.Errorf("cart not found or expired")
	}
	if err != nil {
		logger.Errorf("Failed to get cart: %v", err)
		return fmt.Errorf("failed to get cart: %w", err)
	}

	// Update last activity
	c, err = h.EntClient.Cart.UpdateOneID(c.ID).
		SetLastActivityAt(time.Now()).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		Save(ctx)
	if err != nil {
		logger.Errorf("Failed to update cart activity: %v", err)
		return fmt.Errorf("failed to update cart: %w", err)
	}

	rsp.Cart = toProtoCart(c)
	logger.Infof("Cart fetched successfully: %s", c.ID)
	return nil
}

// AddCartItem adds an item to the cart, merging quantities if the product exists
func (h *CartService) AddCartItem(ctx context.Context, req *pb.AddCartItemRequest, rsp *pb.AddCartItemResponse) error {
	logger.Infof("Received AddCartItem request for cart_id: %s, product_id: %s", req.CartId, req.ProductId)

	if req.Quantity <= 0 {
		logger.Infof("Invalid quantity: %d", req.Quantity)
		return fmt.Errorf("quantity must be positive")
	}

	cartID, err := uuid.Parse(req.CartId)
	if err != nil {
		logger.Errorf("Invalid cart_id format: %v", err)
		return fmt.Errorf("invalid cart_id format: %w", err)
	}

	// Start a transaction
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		logger.Errorf("Failed to start transaction: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify cart exists and is active
	_, err = tx.Cart.Query().
		Where(
			cart.ID(cartID),
			cart.DeletedAtIsNil(),
			cart.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found or expired: %s", req.CartId)
		return fmt.Errorf("cart not found or expired")
	}
	if err != nil {
		logger.Errorf("Failed to query cart: %v", err)
		return fmt.Errorf("failed to query cart: %w", err)
	}

	// Check if product already exists in cart
	existingItem, err := tx.CartItem.Query().
		Where(
			cartitem.HasCartWith(cart.ID(cartID)),
			cartitem.ProductID(uuid.MustParse(req.ProductId)),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		logger.Errorf("Failed to query cart item: %v", err)
		return fmt.Errorf("failed to query cart item: %w", err)
	}

	if existingItem != nil {
		// Update quantity
		err = tx.CartItem.UpdateOneID(existingItem.ID).
			AddQuantity(int(req.Quantity)).
			SetUpdatedAt(time.Now()).
			Exec(ctx)
		if err != nil {
			logger.Errorf("Failed to update cart item quantity: %v", err)
			return fmt.Errorf("failed to update cart item: %w", err)
		}
	} else {
		// Create new cart item
		_, err = tx.CartItem.Create().
			SetCartID(cartID).
			SetProductID(uuid.MustParse(req.ProductId)).
			SetQuantity(int(req.Quantity)).
			Save(ctx)
		if err != nil {
			logger.Errorf("Failed to create cart item: %v", err)
			return fmt.Errorf("failed to create cart item: %w", err)
		}
	}

	// Update cart metadata
	err = tx.Cart.UpdateOneID(cartID).
		SetLastActivityAt(time.Now()).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		AddVersion(1).
		Exec(ctx)
	if err != nil {
		logger.Errorf("Failed to update cart metadata: %v", err)
		return fmt.Errorf("failed to update cart: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		logger.Errorf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated cart
	cWithItems, err := h.EntClient.Cart.Query().
		Where(cart.ID(cartID)).
		WithCartItems().
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch updated cart: %v", err)
		return fmt.Errorf("failed to fetch updated cart: %w", err)
	}

	rsp.Cart = toProtoCart(cWithItems)
	logger.Infof("Added item to cart: %s", req.CartId)
	return nil
}

// UpdateCartItem updates the quantity of a cart item
func (h *CartService) UpdateCartItem(ctx context.Context, req *pb.UpdateCartItemRequest, rsp *pb.UpdateCartItemResponse) error {
	logger.Infof("Received UpdateCartItem request for cart_id: %s, cart_item_id: %s", req.CartId, req.CartItemId)

	if req.Quantity <= 0 {
		logger.Infof("Invalid quantity: %d", req.Quantity)
		return fmt.Errorf("quantity must be positive")
	}

	cartID, err := uuid.Parse(req.CartId)
	if err != nil {
		logger.Errorf("Invalid cart_id format: %v", err)
		return fmt.Errorf("invalid cart_id format: %w", err)
	}

	// Start a transaction
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		logger.Errorf("Failed to start transaction: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify cart exists and version matches
	_, err = tx.Cart.Query().
		Where(
			cart.ID(cartID),
			cart.Version(int(req.Version)),
			cart.DeletedAtIsNil(),
			cart.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found, expired, or version mismatch: %s", req.CartId)
		return fmt.Errorf("cart not found, expired, or version mismatch")
	}
	if err != nil {
		logger.Errorf("Failed to query cart: %v", err)
		return fmt.Errorf("failed to query cart: %w", err)
	}

	// Update cart item
	err = tx.CartItem.UpdateOneID(uuid.MustParse(req.CartItemId)).
		SetQuantity(int(req.Quantity)).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart item not found: %s", req.CartItemId)
		return fmt.Errorf("cart item not found")
	}
	if err != nil {
		logger.Errorf("Failed to update cart item: %v", err)
		return fmt.Errorf("failed to update cart item: %w", err)
	}

	// Update cart metadata
	err = tx.Cart.UpdateOneID(cartID).
		SetLastActivityAt(time.Now()).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		AddVersion(1).
		Exec(ctx)
	if err != nil {
		logger.Errorf("Failed to update cart metadata: %v", err)
		return fmt.Errorf("failed to update cart: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		logger.Errorf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated cart
	cWithItems, err := h.EntClient.Cart.Query().
		Where(cart.ID(cartID)).
		WithCartItems().
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch updated cart: %v", err)
		return fmt.Errorf("failed to fetch updated cart: %w", err)
	}

	rsp.Cart = toProtoCart(cWithItems)
	logger.Infof("Updated cart item: %s in cart: %s", req.CartItemId, req.CartId)
	return nil
}

// RemoveCartItem removes a cart item
func (h *CartService) RemoveCartItem(ctx context.Context, req *pb.RemoveCartItemRequest, rsp *pb.RemoveCartItemResponse) error {
	logger.Infof("Received RemoveCartItem request for cart_id: %s, cart_item_id: %s", req.CartId, req.CartItemId)

	cartID, err := uuid.Parse(req.CartId)
	if err != nil {
		logger.Errorf("Invalid cart_id format: %v", err)
		return fmt.Errorf("invalid cart_id format: %w", err)
	}

	// Start a transaction
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		logger.Errorf("Failed to start transaction: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify cart exists and version matches
	_, err = tx.Cart.Query().
		Where(
			cart.ID(cartID),
			cart.Version(int(req.Version)),
			cart.DeletedAtIsNil(),
			cart.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found, expired, or version mismatch: %s", req.CartId)
		return fmt.Errorf("cart not found, expired, or version mismatch")
	}
	if err != nil {
		logger.Errorf("Failed to query cart: %v", err)
		return fmt.Errorf("failed to query cart: %w", err)
	}

	// Delete cart item
	err = tx.CartItem.DeleteOneID(uuid.MustParse(req.CartItemId)).Exec(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart item not found: %s", req.CartItemId)
		return fmt.Errorf("cart item not found")
	}
	if err != nil {
		logger.Errorf("Failed to delete cart item: %v", err)
		return fmt.Errorf("failed to delete cart item: %w", err)
	}

	// Update cart metadata
	err = tx.Cart.UpdateOneID(cartID).
		SetLastActivityAt(time.Now()).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		AddVersion(1).
		Exec(ctx)
	if err != nil {
		logger.Errorf("Failed to update cart metadata: %v", err)
		return fmt.Errorf("failed to update cart: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		logger.Errorf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated cart
	cWithItems, err := h.EntClient.Cart.Query().
		Where(cart.ID(cartID)).
		WithCartItems().
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch updated cart: %v", err)
		return fmt.Errorf("failed to fetch updated cart: %w", err)
	}

	rsp.Cart = toProtoCart(cWithItems)
	logger.Infof("Removed cart item: %s from cart: %s", req.CartItemId, req.CartId)
	return nil
}

// ClearCart removes all items from a cart
func (h *CartService) ClearCart(ctx context.Context, req *pb.ClearCartRequest, rsp *pb.ClearCartResponse) error {
	logger.Infof("Received ClearCart request for cart_id: %s", req.CartId)

	cartID, err := uuid.Parse(req.CartId)
	if err != nil {
		logger.Errorf("Invalid cart_id format: %v", err)
		return fmt.Errorf("invalid cart_id format: %w", err)
	}

	// Start a transaction
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		logger.Errorf("Failed to start transaction: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify cart exists and version matches
	_, err = tx.Cart.Query().
		Where(
			cart.ID(cartID),
			cart.Version(int(req.Version)),
			cart.DeletedAtIsNil(),
			cart.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found, expired, or version mismatch: %s", req.CartId)
		return fmt.Errorf("cart not found, expired, or version mismatch")
	}
	if err != nil {
		logger.Errorf("Failed to query cart: %v", err)
		return fmt.Errorf("failed to query cart: %w", err)
	}

	// Delete all cart items
	_, err = tx.CartItem.Delete().
		Where(cartitem.HasCartWith(cart.ID(cartID))).
		Exec(ctx)
	if err != nil {
		logger.Errorf("Failed to delete cart items: %v", err)
		return fmt.Errorf("failed to delete cart items: %w", err)
	}

	// Update cart metadata
	err = tx.Cart.UpdateOneID(cartID).
		SetLastActivityAt(time.Now()).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		AddVersion(1).
		Exec(ctx)
	if err != nil {
		logger.Errorf("Failed to update cart metadata: %v", err)
		return fmt.Errorf("failed to update cart: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		logger.Errorf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated cart
	cWithItems, err := h.EntClient.Cart.Query().
		Where(cart.ID(cartID)).
		WithCartItems().
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch updated cart: %v", err)
		return fmt.Errorf("failed to fetch updated cart: %w", err)
	}

	rsp.Cart = toProtoCart(cWithItems)
	logger.Infof("Cleared cart: %s", req.CartId)
	return nil
}

// SoftDeleteCart marks a cart as deleted
func (h *CartService) SoftDeleteCart(ctx context.Context, req *pb.SoftDeleteCartRequest, rsp *pb.SoftDeleteCartResponse) error {
	logger.Infof("Received SoftDeleteCart request for ID: %s", req.Id)

	cartID, err := uuid.Parse(req.Id)
	if err != nil {
		logger.Errorf("Invalid cart_id format: %v", err)
		return fmt.Errorf("invalid cart_id format: %w", err)
	}

	// Update cart with deleted_at timestamp
	err = h.EntClient.Cart.UpdateOneID(cartID).
		Where(cart.Version(int(req.Version))).
		SetDeletedAt(time.Now()).
		AddVersion(1).
		Exec(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Cart not found or version mismatch: %s", req.Id)
		rsp.Success = false
		return fmt.Errorf("cart not found or version mismatch")
	}
	if err != nil {
		logger.Errorf("Failed to soft delete cart: %v", err)
		rsp.Success = false
		return fmt.Errorf("failed to soft delete cart: %w", err)
	}

	rsp.Id = req.Id
	rsp.Success = true
	logger.Infof("Cart soft deleted successfully: %s", req.Id)
	return nil
}

// toProtoCart converts an Entgo Cart entity to a Protobuf Cart message
func toProtoCart(c *ent.Cart) *pb.Cart {
	if c == nil {
		return nil
	}
	protoCart := &pb.Cart{
		Id:             c.ID.String(),
		UserId:         c.UserID.String(),
		ExpiresAt:      c.ExpiresAt.Unix(),
		LastActivityAt: c.LastActivityAt.Unix(),
		CreatedAt:      c.CreatedAt.Unix(),
		UpdatedAt:      c.UpdatedAt.Unix(),
		Version:        int32(c.Version),
	}
	if !c.DeletedAt.IsZero() {
		protoCart.DeletedAt = c.DeletedAt.Unix()
	}
	if c.Edges.CartItems != nil {
		protoCart.CartItems = make([]*pb.CartItem, len(c.Edges.CartItems))
		for i, item := range c.Edges.CartItems {
			protoCart.CartItems[i] = &pb.CartItem{
				Id:        item.ID.String(),
				ProductId: item.ProductID.String(),
				Quantity:  int32(item.Quantity),
				CreatedAt: item.CreatedAt.Unix(),
				UpdatedAt: item.UpdatedAt.Unix(),
				CartId:    c.ID.String(),
			}
		}
	}
	return protoCart
}
