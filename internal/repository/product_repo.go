package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"order-service/internal/domain/models"
	domainrepo "order-service/internal/domain/repository"
)

type productRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) domainrepo.ProductRepository {
	return &productRepository{db: db}
}

func (r *productRepository) Create(ctx context.Context, product *models.Product) error {
	if err := r.db.WithContext(ctx).Create(product).Error; err != nil {
		return fmt.Errorf("create product: %w", err)
	}
	return nil
}

func (r *productRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	var product models.Product
	err := r.db.WithContext(ctx).
		Preload("Category").
		First(&product, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find product by id: %w", err)
	}
	return &product, nil
}

func (r *productRepository) List(ctx context.Context, filter domainrepo.ProductFilter) ([]*models.Product, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.Product{}).Preload("Category")

	if filter.CategoryID != nil {
		query = query.Where("category_id = ?", *filter.CategoryID)
	}
	if filter.MinPrice != nil {
		query = query.Where("price >= ?", *filter.MinPrice)
	}
	if filter.MaxPrice != nil {
		query = query.Where("price <= ?", *filter.MaxPrice)
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	var products []*models.Product
	if err := query.Offset(offset).Limit(limit).Find(&products).Error; err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	return products, total, nil
}

func (r *productRepository) Update(ctx context.Context, product *models.Product) error {
	if err := r.db.WithContext(ctx).Save(product).Error; err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	return nil
}

func (r *productRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&models.Product{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	return nil
}

func (r *productRepository) DecreaseStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	result := r.db.WithContext(ctx).
		Model(&models.Product{}).
		Where("id = ? AND stock >= ?", productID, quantity).
		UpdateColumn("stock", gorm.Expr("stock - ?", quantity))
	if result.Error != nil {
		return fmt.Errorf("decrease stock: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("insufficient stock for product %s", productID)
	}
	return nil
}

func (r *productRepository) IncreaseStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	if err := r.db.WithContext(ctx).
		Model(&models.Product{}).
		Where("id = ?", productID).
		UpdateColumn("stock", gorm.Expr("stock + ?", quantity)).Error; err != nil {
		return fmt.Errorf("increase stock: %w", err)
	}
	return nil
}
