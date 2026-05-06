package repository

import (
	"context"

	"github.com/google/uuid"

	"order-service/internal/domain/models"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
}

type ProductFilter struct {
	CategoryID *uuid.UUID
	MinPrice   *float64
	MaxPrice   *float64
	Search     string
	Page       int
	Limit      int
}

type ProductRepository interface {
	Create(ctx context.Context, product *models.Product) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error)
	List(ctx context.Context, filter ProductFilter) ([]*models.Product, int64, error)
	Update(ctx context.Context, product *models.Product) error
	Delete(ctx context.Context, id uuid.UUID) error
	DecreaseStock(ctx context.Context, productID uuid.UUID, quantity int) error
	IncreaseStock(ctx context.Context, productID uuid.UUID, quantity int) error
}

type OrderFilter struct {
	UserID *uuid.UUID
	Page   int
	Limit  int
}

type OrderRepository interface {
	Create(ctx context.Context, order *models.Order) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Order, error)
	List(ctx context.Context, filter OrderFilter) ([]*models.Order, int64, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.OrderStatus) error
	WithTx(tx interface{}) OrderRepository
}
