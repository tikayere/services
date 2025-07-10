package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go-micro.dev/v5/logger"

	"products/ent"
	"products/ent/product"
	pb "products/proto"
)

// AdminService implements the AdminServiceServer interface
type AdminService struct {
	EntClient *ent.Client
}

// ForceDeleteProduct handles the forced deletion of a product (admin privilege)
func (h *AdminService) ForceDeleteProduct(ctx context.Context, req *pb.ForceDeleteProductRequest, rsp *pb.ForceDeleteProductResponse) error {
	logger.Infof("Received ForceDeleteProduct request for ID: %s (Admin operation)", req.Id)

	err := h.EntClient.Product.DeleteOneID(uuid.MustParse(req.Id)).Exec(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Product not found for deletion: %s", req.Id)
		rsp.Success = false
		return fmt.Errorf("product not found for deletion: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to force delete product: %v", err)
		rsp.Success = false
		return fmt.Errorf("failed to force delete product: %w", err)
	}

	rsp.Id = req.Id
	rsp.Success = true
	logger.Infof("Product force deleted successfully: %s", req.Id)
	return nil
}

// BulkCreateProducts handles streaming creation of multiple products
func (h *AdminService) BulkCreateProducts(ctx context.Context, stream pb.AdminService_BulkCreateProductsStream) error {
	logger.Infof("Received BulkCreateProducts stream request (Admin operation)")
	var createdProducts []*pb.Product
	var totalCreated int32

	for {
		req := &pb.CreateProductRequest{}
		err := stream.RecvMsg(req)
		if err != nil {
			if err.Error() == "EOF" { // go-micro uses EOF for end of stream
				break
			}
			logger.Errorf("Error receiving from BulkCreateProducts stream: %v", err)
			return fmt.Errorf("error receiving product data: %w", err)
		}

		logger.Infof("Bulk creating product: %s", req.Name)

		// Validate subcategory exists
		_, err = h.EntClient.SubCategory.Get(ctx, uuid.MustParse(req.SubcategoryId))
		if ent.IsNotFound(err) {
			logger.Infof("Subcategory not found: %s", req.SubcategoryId)
			continue
		}
		if err != nil {
			logger.Errorf("Failed to validate subcategory: %v", err)
			continue
		}

		// Start a transaction for each product creation
		tx, err := h.EntClient.Tx(ctx)
		if err != nil {
			logger.Errorf("BulkCreateProducts: Failed to start transaction for %s: %v", req.Name, err)
			continue
		}

		p, err := tx.Product.Create().
			SetName(req.Name).
			SetDescription(req.Description).
			SetPrice(req.Price).
			SetStockQuantity(int(req.StockQuantity)).
			SetUserID(uuid.MustParse(req.UserId)).
			SetSubcategoryID(uuid.MustParse(req.SubcategoryId)).
			Save(ctx)
		if ent.IsConstraintError(err) {
			logger.Errorf("BulkCreateProducts: Constraint violation for product %s: %v", req.Name, err)
			tx.Rollback()
			continue
		}
		if err != nil {
			logger.Errorf("BulkCreateProducts: Failed to create product %s: %v", req.Name, err)
			tx.Rollback()
			continue
		}

		if err = tx.Commit(); err != nil {
			logger.Errorf("BulkCreateProducts: Failed to commit transaction for product %s: %v", p.ID, err)
			continue
		}

		// Fetch product with subcategory
		pWithSubcategory, err := h.EntClient.Product.Query().
			Where(product.ID(p.ID)).
			WithSubcategory(func(q *ent.SubCategoryQuery) {
				q.WithCategory()
			}).
			Only(ctx)
		if err != nil {
			logger.Errorf("BulkCreateProducts: Failed to fetch product with subcategory %s: %v", p.ID, err)
			continue
		}

		createdProducts = append(createdProducts, toProtoProduct(pWithSubcategory))
		totalCreated++
	}

	// Send the final response
	err := stream.SendMsg(&pb.BulkCreateProductsResponse{
		Products: createdProducts,
		Total:    totalCreated,
	})
	if err != nil {
		logger.Errorf("Error sending BulkCreateProducts response: %v", err)
		return fmt.Errorf("failed to send response: %w", err)
	}

	logger.Infof("BulkCreateProducts: Successfully created %d products.", totalCreated)
	return nil
}

// ExportProducts streams all products, optionally filtered and paginated
func (h *AdminService) ExportProducts(ctx context.Context, req *pb.ExportProductsRequest, stream pb.AdminService_ExportProductsStream) error {
	logger.Infof("Received ExportProducts stream request (limit: %d, offset: %d, filter: %s)", req.Limit, req.Offset, req.Filter)

	query := h.EntClient.Product.Query().
		WithSubcategory(func(q *ent.SubCategoryQuery) {
			q.WithCategory()
		})

	if req.Filter != "" {
		filter := "%" + req.Filter + "%"
		query.Where(product.NameContainsFold(filter))
	}

	if req.Limit > 0 {
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		query.Offset(int(req.Offset))
	}

	products, err := query.All(ctx)
	if err != nil {
		logger.Errorf("Failed to retrieve products for export: %v", err)
		return fmt.Errorf("failed to retrieve products for export: %w", err)
	}

	for _, p := range products {
		protoProduct := toProtoProduct(p)
		if err := stream.Send(protoProduct); err != nil {
			logger.Errorf("Error sending product %s during export: %v", p.ID, err)
			return fmt.Errorf("failed to stream product: %w", err)
		}
	}

	logger.Infof("Successfully exported %d products.", len(products))
	return nil
}
