package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"order-service/internal/domain/models"
	domainrepo "order-service/internal/domain/repository"
)

var ErrOrderNotFound = errors.New("order not found")
var ErrOrderForbidden = errors.New("access to this order is forbidden")
var ErrInvalidStatusTransition = errors.New("invalid status transition")
var ErrOrderNotCancellable = errors.New("order cannot be cancelled from its current status")
var ErrInsufficientStock = errors.New("insufficient stock")

type OrderService struct {
	orders   domainrepo.OrderRepository
	products domainrepo.ProductRepository
	db       *gorm.DB
}

func NewOrderService(orders domainrepo.OrderRepository, products domainrepo.ProductRepository, db *gorm.DB) *OrderService {
	return &OrderService{orders: orders, products: products, db: db}
}

type CreateOrderItemInput struct {
	ProductID uuid.UUID
	Quantity  int
}

type CreateOrderInput struct {
	UserID  uuid.UUID
	Address string
	Items   []CreateOrderItemInput
}

type ListOrdersInput struct {
	UserID  *uuid.UUID
	IsAdmin bool
	Page    int
	Limit   int
}

type ListOrdersOutput struct {
	Items []*models.Order
	Total int64
}

func (s *OrderService) CreateOrder(ctx context.Context, input CreateOrderInput) (*models.Order, error) {
	var createdOrder *models.Order

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use a tx-scoped product repo for stock updates.
		txProductRepo := &txProductRepo{db: tx}

		var items []models.OrderItem
		var total float64

		for _, itemInput := range input.Items {
			product, err := s.products.FindByID(ctx, itemInput.ProductID)
			if err != nil {
				return fmt.Errorf("find product %s: %w", itemInput.ProductID, err)
			}
			if product == nil {
				return fmt.Errorf("product %s not found", itemInput.ProductID)
			}
			if product.Stock < itemInput.Quantity {
				return fmt.Errorf("%w: product %s (available: %d, requested: %d)",
					ErrInsufficientStock, product.Name, product.Stock, itemInput.Quantity)
			}

			if err := txProductRepo.decreaseStock(ctx, itemInput.ProductID, itemInput.Quantity); err != nil {
				return err
			}

			items = append(items, models.OrderItem{
				ID:        uuid.New(),
				ProductID: itemInput.ProductID,
				Quantity:  itemInput.Quantity,
				Price:     product.Price,
			})
			total += product.Price * float64(itemInput.Quantity)
		}

		order := &models.Order{
			ID:          uuid.New(),
			UserID:      input.UserID,
			Status:      models.OrderStatusNew,
			Address:     input.Address,
			TotalAmount: total,
			Items:       items,
		}

		if err := tx.Create(order).Error; err != nil {
			return fmt.Errorf("create order: %w", err)
		}

		createdOrder = order
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Reload with associations.
	return s.orders.FindByID(ctx, createdOrder.ID)
}

func (s *OrderService) GetOrder(ctx context.Context, orderID uuid.UUID, requesterID uuid.UUID, isAdmin bool) (*models.Order, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if !isAdmin && order.UserID != requesterID {
		return nil, ErrOrderForbidden
	}
	return order, nil
}

func (s *OrderService) ListOrders(ctx context.Context, input ListOrdersInput) (*ListOrdersOutput, error) {
	filter := domainrepo.OrderFilter{
		Page:  input.Page,
		Limit: input.Limit,
	}
	if !input.IsAdmin && input.UserID != nil {
		filter.UserID = input.UserID
	}

	items, total, err := s.orders.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return &ListOrdersOutput{Items: items, Total: total}, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, orderID uuid.UUID, requesterID uuid.UUID, isAdmin bool) (*models.Order, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("find order: %w", err)
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	if !isAdmin && order.UserID != requesterID {
		return nil, ErrOrderForbidden
	}
	if !models.CancellableStatuses[order.Status] {
		return nil, ErrOrderNotCancellable
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txProductRepo := &txProductRepo{db: tx}

		for _, item := range order.Items {
			if err := txProductRepo.increaseStock(ctx, item.ProductID, item.Quantity); err != nil {
				return err
			}
		}

		return tx.Model(&models.Order{}).
			Where("id = ?", orderID).
			Update("status", models.OrderStatusCancelled).Error
	})
	if err != nil {
		return nil, fmt.Errorf("cancel order: %w", err)
	}

	return s.orders.FindByID(ctx, orderID)
}

func (s *OrderService) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, newStatus models.OrderStatus) (*models.Order, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("find order: %w", err)
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}

	if !models.IsTransitionAllowed(order.Status, newStatus) {
		return nil, fmt.Errorf("%w: %s → %s", ErrInvalidStatusTransition, order.Status, newStatus)
	}

	if newStatus == models.OrderStatusCancelled {
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txProductRepo := &txProductRepo{db: tx}
			for _, item := range order.Items {
				if err := txProductRepo.increaseStock(ctx, item.ProductID, item.Quantity); err != nil {
					return err
				}
			}
			return tx.Model(&models.Order{}).
				Where("id = ?", orderID).
				Update("status", newStatus).Error
		})
	} else {
		err = s.orders.UpdateStatus(ctx, orderID, newStatus)
	}
	if err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	return s.orders.FindByID(ctx, orderID)
}

type txProductRepo struct {
	db *gorm.DB
}

func (r *txProductRepo) decreaseStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	result := r.db.WithContext(ctx).
		Model(&models.Product{}).
		Where("id = ? AND stock >= ?", productID, quantity).
		UpdateColumn("stock", gorm.Expr("stock - ?", quantity))
	if result.Error != nil {
		return fmt.Errorf("decrease stock: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: product %s", ErrInsufficientStock, productID)
	}
	return nil
}

func (r *txProductRepo) increaseStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	return r.db.WithContext(ctx).
		Model(&models.Product{}).
		Where("id = ?", productID).
		UpdateColumn("stock", gorm.Expr("stock + ?", quantity)).Error
}
