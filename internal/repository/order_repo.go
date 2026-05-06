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

type orderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) domainrepo.OrderRepository {
	return &orderRepository{db: db}
}

func (r *orderRepository) WithTx(tx interface{}) domainrepo.OrderRepository {
	return &orderRepository{db: tx.(*gorm.DB)}
}

func (r *orderRepository) Create(ctx context.Context, order *models.Order) error {
	if err := r.db.WithContext(ctx).Create(order).Error; err != nil {
		return fmt.Errorf("create order: %w", err)
	}
	return nil
}

func (r *orderRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	var order models.Order
	err := r.db.WithContext(ctx).
		Preload("Items").
		Preload("Items.Product").
		Preload("User").
		First(&order, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find order by id: %w", err)
	}
	return &order, nil
}

func (r *orderRepository) List(ctx context.Context, filter domainrepo.OrderFilter) ([]*models.Order, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.Order{})

	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
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

	var orders []*models.Order
	err := query.
		Preload("Items").
		Preload("Items.Product").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&orders).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list orders: %w", err)
	}
	return orders, total, nil
}

func (r *orderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.OrderStatus) error {
	if err := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("id = ?", id).
		Update("status", status).Error; err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	return nil
}
