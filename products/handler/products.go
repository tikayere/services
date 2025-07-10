package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go-micro.dev/v5/logger"

	"products/ent"
	"products/ent/category"
	"products/ent/product"
	"products/ent/subcategory"
	pb "products/proto"
)

// ProductService implements the ProductServiceServer interface
type ProductService struct {
	EntClient *ent.Client
}

// CreateProduct handles the creation of a new product
func (h *ProductService) CreateProduct(ctx context.Context, req *pb.CreateProductRequest, rsp *pb.CreateProductResponse) error {
	logger.Infof("Received CreateProduct request for name: %s", req.Name)

	// Validate subcategory exists
	_, err := h.EntClient.SubCategory.Get(ctx, uuid.MustParse(req.SubcategoryId))
	if ent.IsNotFound(err) {
		logger.Infof("Subcategory not found: %s", req.SubcategoryId)
		return fmt.Errorf("subcategory not found")
	}
	if err != nil {
		logger.Errorf("Failed to validate subcategory: %v", err)
		return fmt.Errorf("failed to validate subcategory: %w", err)
	}

	// Create product
	p, err := h.EntClient.Product.Create().
		SetName(req.Name).
		SetDescription(req.Description).
		SetPrice(req.Price).
		SetStockQuantity(int(req.StockQuantity)).
		SetUserID(uuid.MustParse(req.UserId)).
		SetSubcategoryID(uuid.MustParse(req.SubcategoryId)).
		Save(ctx)
	if ent.IsConstraintError(err) {
		logger.Errorf("Constraint violation: %v", err)
		return fmt.Errorf("constraint violation: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to create product: %v", err)
		return fmt.Errorf("failed to create product: %w", err)
	}

	// Fetch product with subcategory
	pWithSubcategory, err := h.EntClient.Product.Query().
		Where(product.ID(p.ID)).
		WithSubcategory(func(q *ent.SubCategoryQuery) {
			q.WithCategory()
		}).
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch product with subcategory: %v", err)
		return fmt.Errorf("failed to fetch product: %w", err)
	}

	rsp.Product = toProtoProduct(pWithSubcategory)
	logger.Infof("Product created successfully: %s", p.ID)
	return nil
}

// GetProduct handles fetching a product by ID
func (h *ProductService) GetProduct(ctx context.Context, req *pb.GetProductRequest, rsp *pb.GetProductResponse) error {
	logger.Infof("Received GetProduct request for ID: %s", req.Id)

	p, err := h.EntClient.Product.Query().
		Where(product.ID(uuid.MustParse(req.Id))).
		WithSubcategory(func(q *ent.SubCategoryQuery) {
			q.WithCategory()
		}).
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Product not found: %s", req.Id)
		return fmt.Errorf("product not found")
	}
	if err != nil {
		logger.Errorf("Failed to get product: %v", err)
		return fmt.Errorf("failed to get product: %w", err)
	}

	rsp.Product = toProtoProduct(p)
	logger.Infof("Product fetched successfully: %s", p.ID)
	return nil
}

// UpdateProduct handles updating an existing product
func (h *ProductService) UpdateProduct(ctx context.Context, req *pb.UpdateProductRequest, rsp *pb.UpdateProductResponse) error {
	logger.Infof("Received UpdateProduct request for ID: %s", req.Id)

	updater := h.EntClient.Product.UpdateOneID(uuid.MustParse(req.Id))

	if req.Name != "" {
		updater.SetName(req.Name)
	}
	if req.Description != "" {
		updater.SetDescription(req.Description)
	}
	if req.Price > 0 {
		updater.SetPrice(req.Price)
	}
	if req.StockQuantity >= 0 {
		updater.SetStockQuantity(int(req.StockQuantity))
	}
	if req.SubcategoryId != "" {
		// Validate subcategory exists
		_, err := h.EntClient.SubCategory.Get(ctx, uuid.MustParse(req.SubcategoryId))
		if ent.IsNotFound(err) {
			logger.Infof("Subcategory not found: %s", req.SubcategoryId)
			return fmt.Errorf("subcategory not found")
		}
		if err != nil {
			logger.Errorf("Failed to validate subcategory: %v", err)
			return fmt.Errorf("failed to validate subcategory: %w", err)
		}
		updater.SetSubcategoryID(uuid.MustParse(req.SubcategoryId))
	}

	p, err := updater.Save(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Product not found for update: %s", req.Id)
		return fmt.Errorf("product not found")
	}
	if ent.IsConstraintError(err) {
		logger.Errorf("Constraint violation during update: %v", err)
		return fmt.Errorf("constraint violation: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to update product: %v", err)
		return fmt.Errorf("failed to update product: %w", err)
	}

	// Fetch product with subcategory
	pWithSubcategory, err := h.EntClient.Product.Query().
		Where(product.ID(p.ID)).
		WithSubcategory(func(q *ent.SubCategoryQuery) {
			q.WithCategory()
		}).
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch product with subcategory: %v", err)
		return fmt.Errorf("failed to fetch product: %w", err)
	}

	rsp.Product = toProtoProduct(pWithSubcategory)
	logger.Infof("Product updated successfully: %s", p.ID)
	return nil
}

// ListProducts handles listing all products with pagination
func (h *ProductService) ListProducts(ctx context.Context, req *pb.ListProductsRequest, rsp *pb.ListProductsResponse) error {
	logger.Infof("Received ListProducts request (limit: %d, offset: %d)", req.Limit, req.Offset)

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
		logger.Errorf("Failed to list products: %v", err)
		return fmt.Errorf("failed to list products: %w", err)
	}

	total, err := h.EntClient.Product.Query().Count(ctx)
	if err != nil {
		logger.Errorf("Failed to count products: %v", err)
		return fmt.Errorf("failed to count products: %w", err)
	}

	protoProducts := make([]*pb.Product, len(products))
	for i, p := range products {
		protoProducts[i] = toProtoProduct(p)
	}

	rsp.Products = protoProducts
	rsp.Total = int32(total)
	logger.Infof("Listed %d products (total: %d)", len(protoProducts), total)
	return nil
}

// SearchProducts searches products by query string
func (h *ProductService) SearchProducts(ctx context.Context, req *pb.SearchProductsRequest, rsp *pb.SearchProductsResponse) error {
	logger.Infof("Received SearchProducts request (query: %s, limit: %d, offset: %d)", req.Query, req.Limit, req.Offset)

	query := h.EntClient.Product.Query().
		WithSubcategory(func(q *ent.SubCategoryQuery) {
			q.WithCategory()
		})

	if req.Query != "" {
		searchStr := "%" + req.Query + "%"
		query.Where(product.Or(
			product.NameContainsFold(searchStr),
			product.DescriptionContainsFold(searchStr),
		))
	}

	if req.Limit > 0 {
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		query.Offset(int(req.Offset))
	}

	products, err := query.All(ctx)
	if err != nil {
		logger.Errorf("Failed to search products: %v", err)
		return fmt.Errorf("failed to search products: %w", err)
	}

	total, err := h.EntClient.Product.Query().Count(ctx)
	if err != nil {
		logger.Errorf("Failed to count products for search: %v", err)
		return fmt.Errorf("failed to count products for search: %w", err)
	}

	protoProducts := make([]*pb.Product, len(products))
	for i, p := range products {
		protoProducts[i] = toProtoProduct(p)
	}

	rsp.Products = protoProducts
	rsp.Total = int32(total)
	logger.Infof("Found %d products matching query '%s' (total: %d)", len(protoProducts), req.Query, total)
	return nil
}

// CreateCategory handles the creation of a new category
func (h *ProductService) CreateCategory(ctx context.Context, req *pb.CreateCategoryRequest, rsp *pb.CreateCategoryResponse) error {
	logger.Infof("Received CreateCategory request for name: %s", req.Name)

	c, err := h.EntClient.Category.Create().
		SetName(req.Name).
		SetDescription(req.Description).
		Save(ctx)
	if ent.IsConstraintError(err) {
		logger.Errorf("Constraint violation: %v", err)
		return fmt.Errorf("constraint violation: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to create category: %v", err)
		return fmt.Errorf("failed to create category: %w", err)
	}

	rsp.Category = toProtoCategory(c)
	logger.Infof("Category created successfully: %s", c.ID)
	return nil
}

// GetCategory handles fetching a category by ID
func (h *ProductService) GetCategory(ctx context.Context, req *pb.GetCategoryRequest, rsp *pb.GetCategoryResponse) error {
	logger.Infof("Received GetCategory request for ID: %s", req.Id)

	c, err := h.EntClient.Category.Query().
		Where(category.ID(uuid.MustParse(req.Id))).
		WithSubcategories().
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Category not found: %s", req.Id)
		return fmt.Errorf("category not found")
	}
	if err != nil {
		logger.Errorf("Failed to get category: %v", err)
		return fmt.Errorf("failed to get category: %w", err)
	}

	rsp.Category = toProtoCategory(c)
	logger.Infof("Category fetched successfully: %s", c.ID)
	return nil
}

// CreateSubcategory handles the creation of a new subcategory
func (h *ProductService) CreateSubcategory(ctx context.Context, req *pb.CreateSubcategoryRequest, rsp *pb.CreateSubcategoryResponse) error {
	logger.Infof("Received CreateSubcategory request for name: %s", req.Name)

	// Validate category exists
	_, err := h.EntClient.Category.Get(ctx, uuid.MustParse(req.CategoryId))
	if ent.IsNotFound(err) {
		logger.Infof("Category not found: %s", req.CategoryId)
		return fmt.Errorf("category not found")
	}
	if err != nil {
		logger.Errorf("Failed to validate category: %v", err)
		return fmt.Errorf("failed to validate category: %w", err)
	}

	sc, err := h.EntClient.SubCategory.Create().
		SetName(req.Name).
		SetDescription(req.Description).
		SetCategoryID(uuid.MustParse(req.CategoryId)).
		Save(ctx)
	if ent.IsConstraintError(err) {
		logger.Errorf("Constraint violation: %v", err)
		return fmt.Errorf("constraint violation: %w", err)
	}
	if err != nil {
		logger.Errorf("Failed to create subcategory: %v", err)
		return fmt.Errorf("failed to create subcategory: %w", err)
	}

	// Fetch subcategory with category
	scWithCategory, err := h.EntClient.SubCategory.Query().
		Where(subcategory.ID(sc.ID)).
		WithCategory().
		Only(ctx)
	if err != nil {
		logger.Errorf("Failed to fetch subcategory with category: %v", err)
		return fmt.Errorf("failed to fetch subcategory: %w", err)
	}

	rsp.Subcategory = toProtoSubcategory(scWithCategory)
	logger.Infof("Subcategory created successfully: %s", sc.ID)
	return nil
}

// GetSubcategory handles fetching a subcategory by ID
func (h *ProductService) GetSubcategory(ctx context.Context, req *pb.GetSubcategoryRequest, rsp *pb.GetSubcategoryResponse) error {
	logger.Infof("Received GetSubcategory request for ID: %s", req.Id)

	sc, err := h.EntClient.SubCategory.Query().
		Where(subcategory.ID(uuid.MustParse(req.Id))).
		WithCategory().
		Only(ctx)
	if ent.IsNotFound(err) {
		logger.Infof("Subcategory not found: %s", req.Id)
		return fmt.Errorf("subcategory not found")
	}
	if err != nil {
		logger.Errorf("Failed to get subcategory: %v", err)
		return fmt.Errorf("failed to get subcategory: %w", err)
	}

	rsp.Subcategory = toProtoSubcategory(sc)
	logger.Infof("Subcategory fetched successfully: %s", sc.ID)
	return nil
}

// toProtoProduct converts an Entgo Product entity to a Protobuf Product message
func toProtoProduct(p *ent.Product) *pb.Product {
	if p == nil {
		return nil
	}
	protoProduct := &pb.Product{
		Id:            p.ID.String(),
		Name:          p.Name,
		Description:   *p.Description,
		Price:         p.Price,
		StockQuantity: int32(p.StockQuantity),
		UserId:        p.UserID.String(),
		CreatedAt:     p.CreatedAt.Unix(),
		UpdatedAt:     p.UpdatedAt.Unix(),
		IsActive:      p.IsActive,
	}
	if p.Edges.Subcategory != nil {
		protoProduct.Subcategory = toProtoSubcategory(p.Edges.Subcategory)
	}
	return protoProduct
}

// toProtoCategory converts an Entgo Category entity to a Protobuf Category message
func toProtoCategory(c *ent.Category) *pb.Category {
	if c == nil {
		return nil
	}
	protoCategory := &pb.Category{
		Id:          c.ID.String(),
		Name:        c.Name,
		Description: *c.Description,
		CreatedAt:   c.CreatedAt.Unix(),
		UpdatedAt:   c.UpdatedAt.Unix(),
	}
	if c.Edges.Subcategories != nil {
		protoCategory.Subcategories = make([]*pb.Subcategory, len(c.Edges.Subcategories))
		for i, sc := range c.Edges.Subcategories {
			protoCategory.Subcategories[i] = toProtoSubcategory(sc)
		}
	}
	return protoCategory
}

// toProtoSubcategory converts an Entgo Subcategory entity to a Protobuf Subcategory message
func toProtoSubcategory(sc *ent.SubCategory) *pb.Subcategory {
	if sc == nil {
		return nil
	}
	protoSubcategory := &pb.Subcategory{
		Id:          sc.ID.String(),
		Name:        sc.Name,
		Description: *sc.Description,
		CreatedAt:   sc.CreatedAt.Unix(),
		UpdatedAt:   sc.UpdatedAt.Unix(),
	}
	if sc.Edges.Category != nil {
		protoSubcategory.Category = toProtoCategory(sc.Edges.Category)
	}
	return protoSubcategory
}
